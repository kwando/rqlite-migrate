# rqlite-migrate

A small, dbmate-style migration CLI for rqlite. Requires Go 1.22 or newer to build.

## Usage

```sh
go build -o rqlite-migrate .
export RQLITE_URL=http://localhost:4001
./rqlite-migrate new create_users
./rqlite-migrate up
./rqlite-migrate status
./rqlite-migrate down
./rqlite-migrate --version
```

`rqlite-migrate --version` prints the release tag and short Git commit in published binaries (for example, `rqlite-migrate v0.0.2 (abc1234)`). Local builds print `dev` with the short commit when available, plus `-dirty` if the working tree has uncommitted changes.

### Connection and authentication

Set `RQLITE_URL` (or pass `--url` before the command) to the rqlite HTTP API base URL. For servers using HTTP Basic Auth, include the username and password in the URL:

```sh
RQLITE_URL='https://migrator:YOUR_PASSWORD@rqlite.example.com:4001' rqlite-migrate status
```

URL-encode special characters in the username or password (for example, `@` as `%40`). Use HTTPS to protect credentials in transit, and inject the URL through a secret environment variable rather than saving a real password in source control or shell history. The account needs rqlite's `query` permission for `status`, and both `query` and `execute` for `up` and `down`. TLS uses the system's trusted certificates; custom CA and client-certificate options are not currently supported.

### Migrations

The `--url` and `--dir` flags go before the command. The default directory is `db/migrations`, relative to the directory where you run the CLI (not the binary's location). Use `--dir /absolute/path/to/db/migrations` when running from elsewhere. `new` creates the directory; `status` reports when it is missing, while `up` and `down` require it. Files are named `YYYYMMDDHHMMSS_name.sql` (UTC), and use this format:

```sql
-- migrate:up
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- migrate:down
DROP TABLE users;
```

Each migration is sent as one `/db/execute?transaction` request, with its `schema_migrations` history update in the same request. `down` reverts the latest applied migration and requires a nonempty down section. `status` does not modify the database. Run only one migration process per cluster at a time; the tool does not implement a distributed lock. Avoid manually issuing `BEGIN`, `COMMIT`, or `ROLLBACK` in migrations, since rqlite does not define their behavior.

## Development

Run unit tests with `go test ./...`. To run integration tests, install `rqlited` on your `PATH` and run `go test -tags integration -count=1 ./...`. Each test starts its own temporary single-node rqlite instance; the tests never use `RQLITE_URL`. CI downloads rqlite v10.6.0 and runs this suite separately from the unit tests.

## Releases

Push a version tag to run the release workflow (after merging the workflow to the default branch):

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs the real-rqlite integration tests before publishing a GitHub Release with standalone `rqlite-migrate` archives for macOS Apple Silicon (`darwin/arm64`), Linux x86-64 (`linux/amd64`), and Raspberry Pi with a 64-bit OS (`linux/arm64`). It also uploads `SHA256SUMS` for verifying downloads; run `sha256sum -c SHA256SUMS` on Linux or `shasum -a 256 -c SHA256SUMS` on macOS in a directory containing all three archives. There is no macOS Intel or 32-bit Raspberry Pi build.
