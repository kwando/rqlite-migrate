package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientTransactionAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "/db/execute?transaction" {
			t.Errorf("unexpected URL %s", r.URL)
		}
		var statements []statement
		if err := json.NewDecoder(r.Body).Decode(&statements); err != nil {
			t.Error(err)
		}
		if len(statements) != 2 || statements[1][1] != "123" {
			t.Errorf("unexpected statements: %v", statements)
		}
		fmt.Fprint(w, `{"results":[{}, {"error_v2":{"message":"duplicate version"}}]}`)
	}))
	defer server.Close()
	c, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = c.execute(context.Background(), []statement{{"CREATE TABLE t (id INTEGER)"}, {"INSERT INTO schema_migrations VALUES (?)", "123"}})
	if err == nil || err.Error() != "statement 2: duplicate version" {
		t.Fatalf("unexpected error: %v", err)
	}
}
