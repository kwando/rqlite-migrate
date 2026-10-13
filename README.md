# rqlite-migrate

A small, dbmate-style migration CLI for rqlite. Requires Go 1.22 or newer to build.

```sh
go build -o rqlite-migrate .
export RQLITE_URL=http://localhost:4001
./rqlite-migrate new create_users
./rqlite-migrate up
./rqlite-migrate status
./rqlite-migrate down
```

With [just](https://just.systems/), run `just` to list recipes, `just check` to test and vet, or `just new create_users`, `just status`, `just up`, and `just down`. Set `RQLITE_URL` before using database recipes.

To run integration tests, install `rqlited` on your `PATH` and run `just integration` (or `go test -tags integration -count=1 ./...`). Each test starts its own temporary single-node rqlite instance; the tests never use `RQLITE_URL`. CI downloads rqlite v10.6.0 and runs this suite separately from the unit tests.

## Releases

Push a version tag to run the release workflow (after merging the workflow to the default branch):

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs the real-rqlite integration tests before publishing a GitHub Release with standalone `rqlite-migrate` archives for macOS Apple Silicon (`darwin/arm64`), Linux x86-64 (`linux/amd64`), and Raspberry Pi with a 64-bit OS (`linux/arm64`). It also uploads `SHA256SUMS` for verifying downloads; run `sha256sum -c SHA256SUMS` on Linux or `shasum -a 256 -c SHA256SUMS` on macOS in a directory containing all three archives. There is no macOS Intel or 32-bit Raspberry Pi build.

The `--url` and `--dir` flags go before the command. The default directory is `db/migrations`, relative to the directory where you run the CLI (not the binary's location). Use `--dir /absolute/path/to/db/migrations` when running from elsewhere. `new` creates the directory; `status` reports when it is missing, while `up` and `down` require it. Files are named `YYYYMMDDHHMMSS_name.sql` (UTC), and use this format:

```sql
-- migrate:up
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- migrate:down
DROP TABLE users;
```

Each migration is sent as one `/db/execute?transaction` request, with its `schema_migrations` history update in the same request. `down` reverts the latest applied migration and requires a nonempty down section. `status` does not modify the database. Run only one migration process per cluster at a time; the tool does not implement a distributed lock. Avoid manually issuing `BEGIN`, `COMMIT`, or `ROLLBACK` in migrations, since rqlite does not define their behavior.
