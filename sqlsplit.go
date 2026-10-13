package main

import (
	"fmt"
	"strings"
)

// splitSQL recognizes SQLite strings, comments, identifiers, and trigger bodies.
// It deliberately does not try to validate SQL; rqlite is the SQL authority.
func splitSQL(sql string) ([]statement, error) {
	var out []statement
	start := 0
	hasCode := false
	words := []string{}
	trigger, body, bodyDone, caseDepth := false, false, false, 0
	for i := 0; i < len(sql); {
		c := sql[i]
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i += 2
			end := strings.Index(sql[i:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated SQL comment")
			}
			i += end + 2
			continue
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			hasCode = true
			close := c
			if c == '[' {
				close = ']'
			}
			i++
			closed := false
			for i < len(sql) {
				if sql[i] == close {
					if close != ']' && i+1 < len(sql) && sql[i+1] == close {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQL quote")
			}
			continue
		}
		if isWord(c) {
			begin := i
			for i < len(sql) && isWord(sql[i]) {
				i++
			}
			word := strings.ToUpper(sql[begin:i])
			hasCode = true
			if len(words) < 3 {
				words = append(words, word)
				if (len(words) == 2 && words[0] == "CREATE" && words[1] == "TRIGGER") ||
					(len(words) == 3 && words[0] == "CREATE" && (words[1] == "TEMP" || words[1] == "TEMPORARY") && words[2] == "TRIGGER") {
					trigger = true
				}
			}
			if trigger {
				switch {
				case word == "BEGIN" && !body:
					body = true
				case body && word == "CASE":
					caseDepth++
				case body && word == "END" && caseDepth > 0:
					caseDepth--
				case body && word == "END":
					bodyDone = true
				}
			}
			continue
		}
		if c == ';' && (!trigger || bodyDone) {
			if hasCode {
				out = append(out, statement{strings.TrimSpace(sql[start : i+1])})
			}
			start, hasCode = i+1, false
			words = nil
			trigger, body, bodyDone, caseDepth = false, false, false, 0
		} else if c != ';' && c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			hasCode = true
		}
		i++
	}
	if trigger && body && !bodyDone {
		return nil, fmt.Errorf("unterminated trigger body")
	}
	if hasCode {
		out = append(out, statement{strings.TrimSpace(sql[start:])})
	}
	return out, nil
}

func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
