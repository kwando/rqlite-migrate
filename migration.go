package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var fileName = regexp.MustCompile(`^([0-9]{14})_([a-zA-Z0-9_-]+)\.sql$`)
var migrationName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

type migration struct {
	version string
	name    string
	up      []statement
	down    []statement
}

func newMigration(dir, name string, now time.Time) (string, error) {
	if !migrationName.MatchString(name) {
		return "", fmt.Errorf("invalid migration name %q", name)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, now.Format("20060102150405")+"_"+name+".sql")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	defer file.Close()
	_, err = file.WriteString("-- migrate:up\n\n-- migrate:down\n")
	return path, err
}

func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var migrations []migration
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := fileName.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		if seen[match[1]] {
			return nil, fmt.Errorf("duplicate migration version %s", match[1])
		}
		seen[match[1]] = true
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		up, down, err := parseMigration(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{match[1], entry.Name(), up, down})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

func parseMigration(content string) ([]statement, []statement, error) {
	var up, down strings.Builder
	section := ""
	upSeen, downSeen := false, false
	for _, line := range strings.SplitAfter(content, "\n") {
		switch strings.TrimSpace(line) {
		case "-- migrate:up":
			if upSeen || downSeen {
				return nil, nil, fmt.Errorf("unexpected up directive")
			}
			upSeen, section = true, "up"
		case "-- migrate:down":
			if !upSeen || downSeen {
				return nil, nil, fmt.Errorf("unexpected down directive")
			}
			downSeen, section = true, "down"
		default:
			if section == "" && strings.TrimSpace(line) != "" {
				return nil, nil, fmt.Errorf("content before up directive")
			}
			if section == "up" {
				up.WriteString(line)
			} else if section == "down" {
				down.WriteString(line)
			}
		}
	}
	if !upSeen || !downSeen {
		return nil, nil, fmt.Errorf("expected one up and one down directive")
	}
	u, err := splitSQL(up.String())
	if err != nil {
		return nil, nil, err
	}
	d, err := splitSQL(down.String())
	if err != nil {
		return nil, nil, err
	}
	if len(u) == 0 {
		return nil, nil, fmt.Errorf("up section is empty")
	}
	return u, d, nil
}
