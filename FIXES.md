# seeder: prioritized fix plan

- **Date:** 2026-11-06
- **Commit audited:** `853ceaf62dfc7b5363be82e836570415384945d0` (feat: exclude for column (#24)), fork `jwanglof/seeder`
- **Scope:** git-tracked files only (`git ls-files`, about 5.1k lines of non-test Go)

## Summary

The codebase is in good shape. `go vet ./...` is clean, unit tests pass (`go test ./...`), there are no goroutines and so no data races, and identifiers are quoted properly everywhere: `pgx.Identifier{}.Sanitize()` on Postgres and backtick-doubling via `quoteMySQLIdent` on MySQL. Values always go through COPY or placeholders, and SQL-output literals are escaped. I found no SQL injection and no committed secrets (the compose/CI passwords are throwaway test values). The real risks are about data safety and correctness:
- On Postgres, `--truncate` cascades into tables the user did not select.
- Nothing runs in a transaction, so a failure part-way leaves the database truncated and only partly seeded.
- Unique-integer generation collides on every append re-run and on every `--stream` tick.
- Several schema features are not modelled and fail at insert time: Postgres unique indexes, numeric precision, and integer widths in name rules.
- In `--stream` mode, Ctrl-C is not handled.

Integration tests (`-tags=integration`) were not run because they need live databases.

## Fixes, in order

### 1. Postgres `--truncate` uses `CASCADE` and wipes tables outside the selection (High)
- **Location:** `internal/insert/postgres.go:103`; the same statement is emitted in `internal/insert/sql_output.go:198`. MySQL counterpart: `internal/insert/mysql.go:211-231`.
- **What is wrong:** `TRUNCATE <selected> RESTART IDENTITY CASCADE` also truncates every table that has an FK into the selected set, and resets their sequences. That includes tables the user deliberately left out with `--tables`, `--exclude` or yaml `exclude: true`. The README never mentions the cascade. MySQL has the opposite behaviour: it turns off FK checks and truncates only the selected tables, which leaves dangling child rows. The two backends behave inconsistently.
- **Failure scenario:** `seeder $DSN --tables user --truncate` on the cribcake schema also empties `accommodation_picture`, `report`, `email_change_register`, `password_change_register`, `profile_invite` and the other tables that reference `user`, including tables listed under `exclude:`. That data is lost without warning.
- **Fix:** Before truncating, compute the transitive set of tables that reference the selection. If any of them is outside the selection, abort with the list, or require an explicit `--truncate-cascade`. Otherwise drop `CASCADE`, so Postgres fails safely. Make MySQL do the same check rather than turning off FK checks without warning.

### 2. Truncate and insert are not transactional; a failure leaves a half-seeded database (High)
- **Location:** `internal/insert/insert.go:157-174` (truncate, then one autocommitted `BulkInsert` per batch per table); `internal/insert/postgres.go:95-118`; `internal/insert/stream.go:63-92`.
- **What is wrong:** The truncate commits on its own, and each COPY or INSERT batch commits on its own. A late error does not roll back the earlier work. Examples of late errors: a unique violation, a numeric overflow, or a NOT NULL violation on an excluded column (see #3, #5, #8).
- **Failure scenario:** `--truncate` with 10 tables, where table 7 fails. The database is left with tables 1–6 freshly seeded, the rest empty, and all previous data gone. The exit code is 1, but nothing can be recovered.
- **Fix:** On Postgres, run truncate and all COPYs inside one `pgx.Tx`, with an opt-out flag for huge runs. On MySQL, `TRUNCATE` is DDL and commits implicitly: either use `DELETE` inside a transaction when `--truncate` is set, or document that MySQL runs are not atomic. Also do more preflight validation (#5, #8, #9) so errors surface before any write.

### 3. Unique-integer generator collides on every append re-run and every `--stream` tick (High)
- **Location:** `internal/infer/infer.go:285-322` (`uniqueIntStart`, counter); `internal/insert/stream.go:115-124` (generators rebuilt each tick); `internal/insert/insert.go:415`.
- **What is wrong:**
  - The counter for a non-identity UNIQUE integer column starts at `f.Number(1,1000)`, or at `1` for tinyint and smallint. It takes no account of rows already in the table.
  - With 1000 rows per run, the intervals of two runs (`[a,a+999]` and `[b,b+999]` with `a,b ∈ [1,1000]`) always overlap.
  - Stream mode calls `planColumns` again on every tick, so the counter restarts each time. For smallint and tinyint columns the second tick is guaranteed to repeat values.
  - tinyint and smallint counters also run past the type maximum when the row count is above 127 or 32767.
  - The README claims integers are "widened to a much larger range" (README.md:340), which does not match the code.
- **Failure scenario:** Running `seeder $DSN` twice in append mode on a table with `code int UNIQUE` fails the second run with 23505 or 1062. `--stream` on such a table dies on tick 2.
- **Fix:** In DB mode, start the counter at `SELECT COALESCE(MAX(col),0)+1`, which the driver can expose. In stream mode, build the column plans once and keep the generators across ticks. Run a preflight check that `rows` fits the type's range.

### 4. Postgres unique indexes are not detected (Medium)
- **Location:** `internal/introspect/postgres.go:286-298` (`pgUniquesQuery` reads only `information_schema.table_constraints`).
- **What is wrong:** Uniqueness enforced by `CREATE UNIQUE INDEX` with no constraint is invisible. This is the default for Rails `add_index ..., unique: true` and common elsewhere. Such columns get the plain, non-unique generator, and composite unique indexes are not deduplicated. MySQL is not affected, because unique keys appear in `table_constraints`.
- **Failure scenario:** A table with `CREATE UNIQUE INDEX ON users(username)` gets duplicate `f.Name()` values, and COPY fails with 23505 part-way through (and #2 makes that worse).
- **Fix:** Also query `pg_index` with `indisunique AND NOT indisprimary`, joined to `pg_attribute` for the column list. Skip or handle partial and expression indexes explicitly.

### 5. Generated values ignore integer width and numeric precision (Medium)
- **Location:** `internal/infer/infer.go:205-220` (Money `f.Number(1,100000)`, Count `0..1000`), which is applied before the width-aware `IntForColumn` at `infer.go:273-280`; `internal/generator/generator.go:18-20` (floats `0..100000`); introspection does not read `numeric_precision` or `numeric_scale` (`postgres.go:116-134`, `mysql.go:84-101`).
- **What is wrong:** The name rules return values without checking the column type. Decimal precision is never introspected.
- **Failure scenario:** `amount smallint` produces 100000, which is out of range. `qty tinyint` on MySQL produces up to 1000. `price numeric(5,2)` gets values up to 100000.0, which triggers "numeric field overflow". In each case the insert fails at the database.
- **Fix:** Clamp rule outputs to `intRangeFor(DataType)`. Introspect precision and scale and bound float and decimal generators to `10^(p-s)-1`.

### 6. `--stream` (and long runs) have no signal handling; Ctrl-C kills the process mid-batch (Medium)
- **Location:** `internal/cli/cli.go:105` (`ctx := context.Background()`) and `cli.go:174-183`.
- **What is wrong:** The README says stream runs "until Ctrl-C" (README.md:188), and `RunStream` and the CLI treat `context.Canceled` as a clean exit. But nothing ever cancels `ctx`, so that path is dead code. SIGINT uses Go's default handler: the process dies without closing the connection, exits non-zero (130), and leaves an in-flight COPY or INSERT to be aborted on the server.
- **Fix:** `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM); defer stop()` in `cli.Run`.

### 7. CLI `--exclude` silently drops yaml `exclude:` tables (Medium)
- **Location:** `internal/cli/cli.go:278-298`.
- **What is wrong:** When `--exclude` is passed, only the CLI list is applied and the yaml exclude list is ignored. The README (README.md:232) describes `--exclude` as "equivalent" for the listed tables, which suggests the two are additive. A user who keeps `exclude: true` for lookup or audit tables in yaml loses that protection as soon as they exclude one more table on the command line. Combined with #1, that can truncate them.
- **Fix:** Use the union of the CLI and yaml exclude lists. Document whether `--tables` is meant to override yaml excludes.

### 8. Column `exclude:` is not validated; excluding PK/FK is silently ignored (Medium)
- **Location:** `internal/cli/cli.go:242-276`.
- **What is wrong:**
  - Excluding a `NOT NULL` column that has no default is accepted. It fails only at insert time, possibly after other tables were written (#2).
  - Excluding a PK or FK column is silently ignored (`protected`, lines 247-262), but the README (README.md:243) says it "can't" be done, which implies an error.
- **Fix:** Add a preflight error for an excluded NOT NULL column without a default, and for exclude on a PK or FK column. Name the column in the message.

### 9. Unsupported Postgres types get a random word and fail COPY (Medium)
- **Location:** `internal/introspect/postgres.go:80-111` (`pgKind`); `internal/generator/generator.go:37-40` (KindUnknown → `f.Word()`).
- **What is wrong:** These types map to `KindUnknown`: `ARRAY`, `inet`, `cidr`, `interval`, `money`, `point`, non-enum `USER-DEFINED` types, and others. They receive a random word. pgx's CopyFrom tries to parse the string as text for the target OID (`pgx/v5 values.go:46-63`), and that fails. Columns are always written, even when nullable or defaulted, so any such column aborts the table.
- **Fix:** For unknown kinds, write NULL when the column is nullable, skip it when it has a default, or otherwise fail preflight with a hint to use a `value:` override. Add generators for the common types (arrays, inet, interval).

### 10. `--output sql` (Postgres) emits explicit serial/identity ids without advancing sequences (Medium)
- **Location:** `internal/insert/insert.go:307-320` (keepDBManaged generates PKs); `internal/insert/sql_output.go:207-248`.
- **What is wrong:** The emitted SQL inserts explicit `id` values, which start at a random offset between 1 and 1000. It never calls `setval`. Postgres sequences do not advance on explicit inserts. MySQL AUTO_INCREMENT does advance, so MySQL is not affected.
- **Failure scenario:** After `psql < seed.sql`, the application's first `INSERT` gets `id = 1` from the sequence and hits a duplicate key. The random start can also collide with rows already in the target table.
- **Fix:** For each identity or serial table, emit `SELECT setval(pg_get_serial_sequence('<t>','<col>'), (SELECT MAX(<col>) FROM <t>));` after its inserts.

### 11. Extra positional arguments are silently ignored (Low)
- **Location:** `internal/cli/cli.go:66-71`.
- **What is wrong:** Only `fs.Arg(0)` is used. In `seeder $DSN --tables a, b` the shell splits off `b`, and it is dropped without an error.
- **Fix:** Return `exit.Usage` when `fs.NArg() > 1`.

### 12. Overrides on FK and polymorphic columns are silently ignored (Low)
- **Location:** `internal/insert/insert.go:288-303` (the FK and polymorphic branches run before the override check).
- **What is wrong:** This is documented (README.md:245), but it happens silently. A user who writes `roles_users.columns.role_name.value: registered` (an FK column) gets no warning that it was ignored. db-seeder is a concrete case.
- **Fix:** Make it a preflight error, or at least print a warning on stderr.

### 13. Schema cache is not tied to the DSN (Low)
- **Location:** `internal/introspect/cache.go:318-338`; `internal/cli/cli.go:390-399`.
- **What is wrong:** `--cache file` is reused for any DSN or schema without a check. Pointing it at a different database seeds against the wrong schema.
- **Fix:** Store a hash of host, database and schema (without the password) in `cachedSchema`, and treat a mismatch as a cache miss.

### 14. Pool refresh loads the entire parent table into memory (Low)
- **Location:** `internal/insert/postgres.go:120-152`, `internal/insert/mysql.go:298-339`, `internal/insert/pool.go:235-260`.
- **What is wrong:** After each table, `SELECT pk/ref cols FROM t` runs with no `LIMIT`. The result is loaded into memory and only then trimmed to 100k. The query has no `ORDER BY`, so the rows kept as the "most recent" are arbitrary.
- **Failure scenario:** Appending to a table with tens of millions of rows uses many GB of RAM, and the run is slow or killed by the OOM killer.
- **Fix:** `ORDER BY <pk> DESC LIMIT <capacity>`, or `TABLESAMPLE`.

### 15. Introspection hardcodes schema `public`, but inserts rely on `search_path` (Low)
- **Location:** `internal/introspect/postgres.go:131,197,242,296,353`; unqualified identifiers in `internal/insert/postgres.go:101,112,129`.
- **What is wrong:** If the role's `search_path` resolves first to another schema that has the same table names, introspection and the writes target different tables. Tables outside `public` cannot be seeded at all.
- **Fix:** Use `current_schema()` or add a `--schema` flag, and schema-qualify the identifiers when writing.

### 16. Release pipeline is not gated on CI and uses an unpinned goreleaser (Low)
- **Location:** `.github/workflows/release.yaml:6-8,26-28`.
- **What is wrong:** A tag push starts the release in parallel with CI, so a release can ship while lint or tests fail. `goreleaser` is set to `version: latest`.
- **Fix:** Add `needs:` on the CI jobs, or trigger the release with `workflow_run` on CI success. Pin the goreleaser version.

## Notes / out of scope
- `plan.BuildWithDeps` (`internal/plan/plan.go:67-75`) rejects every FK cycle, even when one edge is nullable and the cycle could be broken by inserting NULL and updating later. This is a feature gap, not a bug.
- MySQL `year` columns map to `KindTime` and receive a `time.Time` (`internal/introspect/mysql.go`, `mySQLKind`). Whether the driver and server accept that in strict mode needs verification.
- `main.go:25-36` scans every argument for `-v`/`-h`, so a flag value spelled `-h` would print help. This is cosmetic.
- I found no credentials, no unchecked errors on write paths, and no resource leaks: rows and connections are closed, and the cache uses tmp-and-rename.
