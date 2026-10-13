//go:build integration

package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Each test owns a separate, temporary single-node cluster. Never point these
// tests at RQLITE_URL: they intentionally create and drop tables.
func startRqlite(t *testing.T) (*client, string) {
	t.Helper()
	binary, err := exec.LookPath("rqlited")
	if err != nil {
		t.Fatal("integration tests require rqlited on PATH")
	}
	address := freeAddress(t)
	raftAddress := freeAddress(t)
	root := t.TempDir()
	log, err := os.Create(filepath.Join(root, "rqlited.log"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "-http-addr", address, "-raft-addr", raftAddress, filepath.Join(root, "data"))
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		cancel()
		log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		cmd.Wait()
		log.Close()
	})
	baseURL := "http://" + address
	deadline := time.Now().Add(20 * time.Second)
	probe := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := probe.Get(baseURL + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				c, err := newClient(baseURL)
				if err != nil {
					t.Fatal(err)
				}
				return c, filepath.Join(root, "migrations")
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	data, _ := os.ReadFile(filepath.Join(root, "rqlited.log"))
	t.Fatalf("rqlited did not become ready: %s", data)
	return nil, ""
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func writeMigration(t *testing.T, dir, version, name, up, down string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, version+"_"+name+".sql")
	if err := os.WriteFile(path, []byte("-- migrate:up\n"+up+"\n-- migrate:down\n"+down+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func migrationCommand(t *testing.T, c *client, dir, command string) error {
	t.Helper()
	return run([]string{"--url", c.url, "--dir", dir, command})
}

func rows(t *testing.T, c *client, sql string) [][]any {
	t.Helper()
	result, err := c.request(context.Background(), "/db/query", []statement{{sql}})
	if err != nil {
		t.Fatal(err)
	}
	return result.Results[0].Values
}

func assertVersions(t *testing.T, c *client, versions ...string) {
	t.Helper()
	applied, err := c.applied(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(versions) {
		t.Fatalf("applied versions = %v, want %v", applied, versions)
	}
	for _, version := range versions {
		if !applied[version] {
			t.Fatalf("applied versions = %v, missing %s", applied, version)
		}
	}
}

func TestIntegrationLifecycle(t *testing.T) {
	c, dir := startRqlite(t)
	const first, second = "20260101000000", "20260102000000"
	writeMigration(t, dir, second, "trigger", `
CREATE TRIGGER rename_user AFTER INSERT ON users BEGIN
  UPDATE users SET name = CASE WHEN new.name = 'a;b' THEN 'updated' ELSE new.name END WHERE id = new.id;
END;
INSERT INTO users VALUES (1, 'a;b');`, `DROP TRIGGER rename_user;
DELETE FROM users WHERE id = 1;`)
	writeMigration(t, dir, first, "users", "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);", "DROP TABLE users;")
	if err := migrationCommand(t, c, dir, "status"); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='schema_migrations'"); len(got) != 0 {
		t.Fatalf("status created history table: %v", got)
	}
	if err := migrationCommand(t, c, dir, "up"); err != nil {
		t.Fatal(err)
	}
	assertVersions(t, c, first, second)
	if got := rows(t, c, "SELECT name FROM users WHERE id = 1"); len(got) != 1 || got[0][0] != "updated" {
		t.Fatalf("trigger did not fire: %v", got)
	}
	if err := migrationCommand(t, c, dir, "up"); err != nil {
		t.Fatal("up must be idempotent:", err)
	}
	if err := migrationCommand(t, c, dir, "status"); err != nil {
		t.Fatal(err)
	}
	if err := migrationCommand(t, c, dir, "down"); err != nil {
		t.Fatal(err)
	}
	assertVersions(t, c, first)
	if got := rows(t, c, "SELECT count(*) FROM users"); got[0][0] != float64(0) {
		t.Fatalf("down did not remove inserted row: %v", got)
	}
	if err := migrationCommand(t, c, dir, "down"); err != nil {
		t.Fatal(err)
	}
	assertVersions(t, c)
	if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='users'"); len(got) != 0 {
		t.Fatalf("down did not remove table: %v", got)
	}
	if err := migrationCommand(t, c, dir, "down"); err != nil {
		t.Fatal("down with nothing applied must be harmless:", err)
	}
}

func TestIntegrationFailedUpRollsBackAndCanRetry(t *testing.T) {
	c, dir := startRqlite(t)
	const first, second = "20260101000000", "20260102000000"
	writeMigration(t, dir, first, "base", "CREATE TABLE base (id INTEGER);", "DROP TABLE base;")
	path := writeMigration(t, dir, second, "broken", "CREATE TABLE temporary (id INTEGER);\nINSERT INTO missing VALUES (1);", "DROP TABLE temporary;")
	err := migrationCommand(t, c, dir, "up")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected SQL error for missing table, got %v", err)
	}
	assertVersions(t, c, first)
	if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='temporary'"); len(got) != 0 {
		t.Fatalf("failed migration left behind table: %v", got)
	}
	if err := os.WriteFile(path, []byte("-- migrate:up\nCREATE TABLE temporary (id INTEGER);\n-- migrate:down\nDROP TABLE temporary;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := migrationCommand(t, c, dir, "up"); err != nil {
		t.Fatalf("retry after repair: %v", err)
	}
	assertVersions(t, c, first, second)
}

func TestIntegrationFailedDownKeepsSchemaAndHistory(t *testing.T) {
	c, dir := startRqlite(t)
	const version = "20260101000000"
	path := writeMigration(t, dir, version, "items", "CREATE TABLE items (id INTEGER);", "DROP TABLE items;\nINSERT INTO missing VALUES (1);")
	if err := migrationCommand(t, c, dir, "up"); err != nil {
		t.Fatal(err)
	}
	err := migrationCommand(t, c, dir, "down")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected SQL error for missing table, got %v", err)
	}
	assertVersions(t, c, version)
	if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='items'"); len(got) != 1 {
		t.Fatalf("failed down removed table: %v", got)
	}
	if err := os.WriteFile(path, []byte("-- migrate:up\nCREATE TABLE items (id INTEGER);\n-- migrate:down\nDROP TABLE items;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := migrationCommand(t, c, dir, "down"); err != nil {
		t.Fatalf("retry after repair: %v", err)
	}
	assertVersions(t, c)
}

func TestIntegrationHistoryAndFileValidation(t *testing.T) {
	t.Run("missing down", func(t *testing.T) {
		c, dir := startRqlite(t)
		const version = "20260101000000"
		writeMigration(t, dir, version, "items", "CREATE TABLE items (id INTEGER);", "")
		if err := migrationCommand(t, c, dir, "up"); err != nil {
			t.Fatal(err)
		}
		if err := migrationCommand(t, c, dir, "down"); err == nil || !strings.Contains(err.Error(), "no down statements") {
			t.Fatalf("expected missing down error, got %v", err)
		}
		assertVersions(t, c, version)
	})
	t.Run("missing local migration", func(t *testing.T) {
		c, dir := startRqlite(t)
		path := writeMigration(t, dir, "20260101000000", "items", "CREATE TABLE items (id INTEGER);", "DROP TABLE items;")
		if err := migrationCommand(t, c, dir, "up"); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"status", "up", "down"} {
			if err := migrationCommand(t, c, dir, command); err == nil || !strings.Contains(err.Error(), "no local migration file") {
				t.Errorf("%s: expected orphan error, got %v", command, err)
			}
		}
		assertVersions(t, c, "20260101000000")
	})
	t.Run("duplicate versions", func(t *testing.T) {
		c, dir := startRqlite(t)
		writeMigration(t, dir, "20260101000000", "one", "CREATE TABLE one (id INTEGER);", "DROP TABLE one;")
		writeMigration(t, dir, "20260101000000", "two", "CREATE TABLE two (id INTEGER);", "DROP TABLE two;")
		if err := migrationCommand(t, c, dir, "up"); err == nil || !strings.Contains(err.Error(), "duplicate migration version") {
			t.Fatalf("expected duplicate error, got %v", err)
		}
		if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='schema_migrations'"); len(got) != 0 {
			t.Fatalf("invalid files changed database: %v", got)
		}
	})
}

func TestIntegrationHistoryInsertFailureRollsBackDDL(t *testing.T) {
	c, dir := startRqlite(t)
	const version = "20260101000000"
	writeMigration(t, dir, version, "widgets", "CREATE TABLE widgets (id INTEGER);", "DROP TABLE widgets;")
	// Force the final history insert to fail, after the CREATE TABLE succeeds.
	if err := c.execute(context.Background(), []statement{{"CREATE TABLE schema_migrations (version TEXT PRIMARY KEY CHECK (version != '" + version + "'))"}}); err != nil {
		t.Fatal(err)
	}
	if err := migrationCommand(t, c, dir, "up"); err == nil {
		t.Fatal("expected history insert to fail")
	}
	assertVersions(t, c)
	if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='widgets'"); len(got) != 0 {
		t.Fatalf("history insert failure left behind DDL: %v", got)
	}
}

func TestIntegrationHistoryDeleteFailureRollsBackDown(t *testing.T) {
	c, dir := startRqlite(t)
	const version = "20260101000000"
	writeMigration(t, dir, version, "widgets", "CREATE TABLE widgets (id INTEGER);", "DROP TABLE widgets;")
	if err := migrationCommand(t, c, dir, "up"); err != nil {
		t.Fatal(err)
	}
	if err := c.execute(context.Background(), []statement{{"CREATE TRIGGER block_delete BEFORE DELETE ON schema_migrations BEGIN SELECT RAISE(ABORT, 'blocked'); END;"}}); err != nil {
		t.Fatal(err)
	}
	if err := migrationCommand(t, c, dir, "down"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected history delete failure, got %v", err)
	}
	assertVersions(t, c, version)
	if got := rows(t, c, "SELECT name FROM sqlite_master WHERE name='widgets'"); len(got) != 1 {
		t.Fatalf("history delete failure removed table: %v", got)
	}
	if err := c.execute(context.Background(), []statement{{"DROP TRIGGER block_delete"}}); err != nil {
		t.Fatal(err)
	}
	if err := migrationCommand(t, c, dir, "down"); err != nil {
		t.Fatalf("retry after unblocking history: %v", err)
	}
	assertVersions(t, c)
}
