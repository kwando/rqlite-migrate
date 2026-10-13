# Run from the repository root with: vhs docs/demo.tape
# Everything shown runs against a disposable single-node rqlite instance.
Output docs/demo.gif
Require go
Require rqlited
Require curl

Set Shell bash
Set Theme "Catppuccin Mocha"
Set Width 800
Set Height 440
Set FontSize 14
Set TypingSpeed 25ms
Set Framerate 24
Set CursorBlink false

Hide
Type `DEMO_DIR=$(mktemp -d) && go build -o "$DEMO_DIR/rqlite-migrate" .`
Enter
Type `mkdir -p "$DEMO_DIR/db/migrations"; printf '%s\n' '-- migrate:up' 'CREATE TABLE users (id INTEGER PRIMARY KEY);' '' '-- migrate:down' 'DROP TABLE users;' > "$DEMO_DIR/db/migrations/20260101000000_create_users.sql"`
Enter
Type `rqlited -http-addr 127.0.0.1:45981 -raft-addr 127.0.0.1:45982 "$DEMO_DIR/data" > "$DEMO_DIR/rqlited.log" 2>&1 &`
Enter
Type `RQLITED_PID=$!; export RQLITE_URL=http://127.0.0.1:45981 PATH="$DEMO_DIR:$PATH"; until curl -fsS "$RQLITE_URL/readyz" >/dev/null 2>&1; do sleep .2; done; cd "$DEMO_DIR"; clear`
Enter
Show

Type "cat db/migrations/*.sql"
Enter
Sleep 1s
Type "rqlite-migrate status"
Enter
Sleep 1s
Type "rqlite-migrate up"
Enter
Sleep 1s
Type "rqlite-migrate status"
Enter
Sleep 1s
Type "rqlite-migrate down"
Enter
Sleep 2s

Hide
Type `kill "$RQLITED_PID"; cd /; rm -rf "$DEMO_DIR"`
Enter
