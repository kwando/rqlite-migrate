package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestVersionFlag(t *testing.T) {
	t.Setenv("RQLITE_URL", "")
	for _, tc := range []struct {
		version string
	}{
		{"dev"},
		{"v0.0.2"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			previousVersion, previousOutput := version, os.Stdout
			version = tc.version
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout = writer
			defer func() {
				version, os.Stdout = previousVersion, previousOutput
				reader.Close()
				writer.Close()
			}()
			if err := run([]string{"--version"}); err != nil {
				t.Fatal(err)
			}
			writer.Close()
			got, err := io.ReadAll(reader)
			if err != nil || !strings.HasPrefix(string(got), "rqlite-migrate "+tc.version) || !strings.HasSuffix(string(got), "\n") {
				t.Fatalf("--version output = %q, err = %v", got, err)
			}
		})
	}
}

func TestVersionText(t *testing.T) {
	const revision = "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct {
		version  string
		settings []debug.BuildSetting
		want     string
	}{
		{"dev", nil, "rqlite-migrate dev"},
		{"v0.0.2", []debug.BuildSetting{{Key: "vcs.revision", Value: revision}}, "rqlite-migrate v0.0.2 (0123456)"},
		{"dev", []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}, {Key: "vcs.revision", Value: revision}}, "rqlite-migrate dev (0123456-dirty)"},
		{"dev", []debug.BuildSetting{{Key: "vcs.revision", Value: "short"}}, "rqlite-migrate dev"},
	} {
		if got := versionText(tc.version, tc.settings); got != tc.want {
			t.Errorf("versionText(%q, %v) = %q, want %q", tc.version, tc.settings, got, tc.want)
		}
	}
}

func TestNewAndLoad(t *testing.T) {
	dir := t.TempDir()
	path, err := newMigration(dir, "create_users", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "20260102030405_create_users.sql" {
		t.Fatal(path)
	}
	if err := os.WriteFile(path, []byte("-- migrate:up\nCREATE TABLE users (id INTEGER);\nINSERT INTO users VALUES (1);\n-- migrate:down\nDROP TABLE users;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := loadMigrations(dir)
	if err != nil || len(files) != 1 || len(files[0].up) != 2 || len(files[0].down) != 1 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
	if _, err := newMigration(dir, "create_users", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)); err == nil {
		t.Fatal("expected duplicate file error")
	}
}

func TestSplitSQL(t *testing.T) {
	sql := `-- comment ;
CREATE TABLE t (v TEXT);
INSERT INTO t VALUES ('semi;''colon'); /* ignored ; */
CREATE TRIGGER tr AFTER INSERT ON t BEGIN
  UPDATE t SET v = CASE WHEN new.v = 'x;y' THEN 'ok' ELSE 'other' END;
  INSERT INTO t VALUES ('trigger;body');
END;
`
	statements, err := splitSQL(sql)
	if err != nil || len(statements) != 3 {
		t.Fatalf("statements=%q err=%v", statements, err)
	}
	if !strings.Contains(statements[2][0].(string), "trigger;body") {
		t.Fatal(statements[2])
	}
	for _, input := range []string{"SELECT 'broken", "/* broken", "CREATE TRIGGER x AFTER INSERT ON t BEGIN INSERT INTO t VALUES (1);"} {
		if _, err := splitSQL(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestInvalidMigration(t *testing.T) {
	for _, input := range []string{"SELECT 1;", "-- migrate:down\nSELECT 1;", "-- migrate:up\n-- migrate:down", "-- migrate:up\nSELECT 1;\n-- migrate:up\n-- migrate:down"} {
		if _, _, err := parseMigration(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestMissingMigrationDirectoryForUpAndDown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "migrations")
	for _, command := range []string{"up", "down"} {
		err := run([]string{"--url", "http://127.0.0.1:1", "--dir", dir, command})
		if err == nil || !strings.Contains(err.Error(), "migration directory not found: "+dir+"\n  looked in: "+dir+"\n") || !strings.Contains(err.Error(), "rqlite-migrate --dir PATH "+command) || !strings.Contains(err.Error(), "rqlite-migrate new NAME") {
			t.Errorf("%s: expected actionable path error, got %v", command, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("migration directory was created: %v", err)
	}
}
