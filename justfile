# List available recipes.
default:
    @just --list

# Build the CLI in the project directory.
build:
    go build -o rqlite-migrate .

# Format Go sources.
fmt:
    gofmt -w *.go

# Run unit tests.
test:
    go test ./...

# Run isolated tests against a real rqlited binary on PATH.
integration:
    go test -tags integration -count=1 ./...

# Run static checks.
vet:
    go vet ./...

# Run tests and static checks.
check: test vet

# Create a new migration (e.g. just new create_users).
new name:
    go run . new {{ quote(name) }}

# Show migration status (requires RQLITE_URL).
status:
    go run . status

# Apply pending migrations (requires RQLITE_URL).
up:
    go run . up

# Roll back the last migration (requires RQLITE_URL).
down:
    go run . down

install: build
    cp rqlite-migrate ~/.local/bin/
