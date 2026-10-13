package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type statement []any

type client struct {
	url  string
	http *http.Client
}

type result struct {
	Results []struct {
		Error   string `json:"error"`
		ErrorV2 *struct {
			Message string `json:"message"`
		} `json:"error_v2"`
		Values [][]any `json:"values"`
	} `json:"results"`
}

func newClient(raw string) (*client, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("expected rqlite base URL, e.g. http://localhost:4001")
	}
	return &client{strings.TrimRight(raw, "/"), &http.Client{Timeout: 60 * time.Second}}, nil
}

func (c *client) request(ctx context.Context, endpoint string, statements []statement) (*result, error) {
	data, err := json.Marshal(statements)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 10<<20)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(limited, 4096))
		return nil, fmt.Errorf("rqlite HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded result
	if err := json.NewDecoder(limited).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Results) != len(statements) {
		// rqlite stops processing a transaction at the first failing statement.
		for i, item := range decoded.Results {
			if item.ErrorV2 != nil {
				return nil, fmt.Errorf("statement %d: %s", i+1, item.ErrorV2.Message)
			}
			if item.Error != "" {
				return nil, fmt.Errorf("statement %d: %s", i+1, item.Error)
			}
		}
		return nil, fmt.Errorf("rqlite returned %d results for %d statements", len(decoded.Results), len(statements))
	}
	for i, item := range decoded.Results {
		if item.ErrorV2 != nil {
			return nil, fmt.Errorf("statement %d: %s", i+1, item.ErrorV2.Message)
		}
		if item.Error != "" {
			return nil, fmt.Errorf("statement %d: %s", i+1, item.Error)
		}
	}
	return &decoded, nil
}

func (c *client) execute(ctx context.Context, statements []statement) error {
	_, err := c.request(ctx, "/db/execute?transaction", statements)
	return err
}

func (c *client) applied(ctx context.Context) (map[string]bool, error) {
	// Do not create the history table for a read-only status command.
	response, err := c.request(ctx, "/db/query", []statement{{"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'"}})
	if err != nil {
		return nil, err
	}
	versions := make(map[string]bool)
	if len(response.Results[0].Values) == 0 {
		return versions, nil
	}
	response, err = c.request(ctx, "/db/query", []statement{{"SELECT version FROM schema_migrations"}})
	if err != nil {
		return nil, err
	}
	for _, row := range response.Results[0].Values {
		if len(row) != 1 {
			return nil, fmt.Errorf("invalid schema_migrations response")
		}
		version, ok := row[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid migration version in response")
		}
		versions[version] = true
	}
	return versions, nil
}
