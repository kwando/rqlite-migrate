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

The `--url` and `--dir` flags go before the command. The default directory is `db/migrations`. Files are named `YYYYMMDDHHMMSS_name.sql` (UTC), and use this format:

```sql
-- migrate:up
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- migrate:down
DROP TABLE users;
```

Each migration is sent as one `/db/execute?transaction` request, with its `schema_migrations` history update in the same request. `down` reverts the latest applied migration and requires a nonempty down section. `status` does not modify the database. Run only one migration process per cluster at a time; the tool does not implement a distributed lock. Avoid manually issuing `BEGIN`, `COMMIT`, or `ROLLBACK` in migrations, since rqlite does not define their behavior.
