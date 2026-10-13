package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("rqlite-migrate", flag.ContinueOnError)
	url := flags.String("url", os.Getenv("RQLITE_URL"), "rqlite HTTP URL (or RQLITE_URL)")
	dir := flags.String("dir", "db/migrations", "migration directory")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: rqlite-migrate [--url URL] [--dir PATH] {new NAME|status|up|down}")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	pos := flags.Args()
	if len(pos) == 0 {
		flags.Usage()
		return errors.New("missing command")
	}
	if pos[0] == "new" {
		if len(pos) != 2 {
			return errors.New("usage: rqlite-migrate new NAME")
		}
		path, err := newMigration(*dir, pos[1], time.Now().UTC())
		if err == nil {
			fmt.Println(path)
		}
		return err
	}
	if len(pos) != 1 || !strings.Contains("|status|up|down|", "|"+pos[0]+"|") {
		return fmt.Errorf("unknown command: %s", strings.Join(pos, " "))
	}
	if *url == "" {
		return errors.New("set --url or RQLITE_URL")
	}
	migrations, err := loadMigrations(*dir)
	if err != nil {
		return err
	}
	client, err := newClient(*url)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if pos[0] != "status" {
		if err := client.execute(ctx, []statement{{"CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)"}}); err != nil {
			return err
		}
	}
	applied, err := client.applied(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(migrations))
	for _, m := range migrations {
		known[m.version] = true
	}
	for version := range applied {
		if !known[version] {
			return fmt.Errorf("applied version %s has no local migration file", version)
		}
	}
	switch pos[0] {
	case "status":
		for _, m := range migrations {
			state := "pending"
			if applied[m.version] {
				state = "applied"
			}
			fmt.Printf("%-8s %s\n", state, m.name)
		}
	case "up":
		for _, m := range migrations {
			if applied[m.version] {
				continue
			}
			statements := append([]statement{}, m.up...)
			statements = append(statements, statement{"INSERT INTO schema_migrations (version) VALUES (?)", m.version})
			if err := client.execute(ctx, statements); err != nil {
				return fmt.Errorf("applying %s: %w", m.name, err)
			}
			fmt.Println("applied", m.name)
		}
	case "down":
		for i := len(migrations) - 1; i >= 0; i-- {
			m := migrations[i]
			if !applied[m.version] {
				continue
			}
			if len(m.down) == 0 {
				return fmt.Errorf("%s has no down statements", m.name)
			}
			statements := append([]statement{}, m.down...)
			statements = append(statements, statement{"DELETE FROM schema_migrations WHERE version = ?", m.version})
			if err := client.execute(ctx, statements); err != nil {
				return fmt.Errorf("rolling back %s: %w", m.name, err)
			}
			fmt.Println("rolled back", m.name)
			break
		}
	}
	return nil
}
