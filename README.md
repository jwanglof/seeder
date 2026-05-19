# seeder

> Zero-config database seeder — one command, realistic data, no factory code.

`seeder` populates your Postgres database with realistic fake data straight
from the schema. No factory code, no YAML, no AI key. Point it at a DSN and
it figures out the rest: it introspects your tables, infers what each column
should look like from its name and type, and bulk-inserts via `COPY` while
respecting your foreign-key constraints.

```bash
$ seeder postgres://user:pass@localhost:5432/mydb --rows 1000 --truncate --seed 42
seeder: 3 table(s), 3 FK(s)
order:  users -> orders -> comments
mode:   truncate + insert
  users     1000 rows (6.7ms)
  orders    1000 rows (7.0ms)
  comments  1000 rows (9.9ms)
done:   3000 row(s) in 53ms
```

## Why

Every backend project hits the same wall: dev DBs with two rows where prod
has a million. Engineers respond by writing factory code, scripting `INSERT`s,
or maintaining fixtures — all of which rot. `seeder` skips that step:
zero-code, zero-config, single Go binary.

The three core promises:

1. **Zero-code.** No factories, no config file. Just a DSN.
2. **Smart inference.** `email` columns get emails; `created_at` gets
   timestamps in the past year; `order_status` (enum) gets one of its labels.
3. **Referential integrity.** FK dependencies are resolved with a
   topological sort; children always point at real parents.

## Install

```bash
go install github.com/mickamy/seeder@latest
```

Requires Go 1.26+ to build from source. Release binaries (macOS / Linux,
x86_64 / arm64) will land on GitHub Releases.

## Usage

```
seeder <dsn> [flags]

FLAGS:
  --config <file>  Path to seeder.yaml (default: auto-detect ./seeder.yaml)
  --dry-run        Print plan, do not insert
  --exclude string Comma-separated tables to skip (cannot combine with --tables)
  --locale string  Locale for name-rule generators (en, ja; default: en)
  --rows int       Rows per table (default: 1000; overrides yaml when set)
  --seed N         Deterministic RNG seed (>= 0; default: time-based)
  --tables string  Comma-separated tables to include (default: all)
  --truncate       TRUNCATE before insert (default: append)
  --verbose        Print per-column inference decisions
  --version, -v    Print seeder version
  --help, -h       Show this help
```

Both `seeder <dsn> [flags]` and `seeder [flags] <dsn>` are accepted.

### Common scenarios

**Frontend / pagination testing.** Two rows is not enough to test page 100
or the "..." truncation in your table UI.

```bash
seeder $DATABASE_URL --rows 10000
```

**Backend / N+1 hunting.** `EXPLAIN ANALYZE` against 50 rows tells you
nothing. Load a realistic volume and the slow path shows up.

```bash
seeder $DATABASE_URL --tables orders,order_items --rows 1000000
```

**Onboarding.** Replace half a page of seed-script setup with one line:

```markdown
1. make db-up
2. make migrate
3. seeder $DATABASE_URL
```

**Dev DB reset after a migration experiment.**

```bash
dropdb mydb && createdb mydb && goose up && seeder $DATABASE_URL --rows 5000
```

**Reproducible CI test data.**

```yaml
- run: |
    make migrate
    seeder $TEST_DB --rows 1000 --seed 42
    go test -tags=integration ./...
```

**Japanese-locale data.** Swap inferred names, addresses, prefectures, phone numbers, and postal codes for plausible Japanese values.

```bash
seeder $DATABASE_URL --locale ja
```

## Configuration

`seeder` runs zero-config out of the box. When you want to pin row counts or skip specific tables without retyping flags every time, drop a `seeder.yaml` next to where you run the command — it is auto-detected. Use `--config path/to/seeder.yaml` to point at one explicitly.

```yaml
version: 1
rows: 1000
seed: 42
locale: en
truncate: false
tables:
  users:
    rows: 5000
    columns:
      email:
        generator: Email
      bio:
        value: dogfood seed row
  orders:
    rows: 10000
  comments:
    exclude: true
```

Precedence is **CLI flag > seeder.yaml > built-in default**. Setting `--rows N` on the command line replaces yaml's row counts for every table; omit it to let per-table values in `tables.<name>.rows` take effect. A full example with comments lives at [`seeder.example.yaml`](./seeder.example.yaml).

Per-column overrides under `tables.<name>.columns.<col>` bypass inference for a single column. Set exactly one of:

- `generator: <Name>` — force a built-in generator (e.g., `Email`, `UUID`, `Phone`, `PastDate`; `seeder --help` lists them via the preflight error).
- `value: <literal>` — pin the column to a fixed yaml value (string, number, bool).

Foreign-key columns are not overridable: yaml entries for them are ignored and the FK pool is used instead, so children still point at real parents.

The yaml `locale` field is equivalent to the `--locale` flag and follows the same precedence.

Pass `--verbose` to see which inference rule each column matched, e.g., when you are debugging why `bio` ended up with a long paragraph instead of the short string you expected:

```
$ seeder $DATABASE_URL --rows 5 --verbose
seeder: 3 table(s), 2 FK(s)
order:  users -> orders -> comments
mode:   append
  users
    id            skip: identity
    email         name match: Email
    name          name match: Name
    created_at    name match: PastDate
  users  5 rows (1.2ms)
  ...
```

## How it works

### Schema introspection

`seeder` queries `information_schema` and `pg_catalog` for tables, columns,
primary keys, foreign keys, and enum types in the `public` schema. No
schema changes, no privileged access — just standard SELECTs.

### Smart inference

Each column is matched against a small set of name patterns first, then
falls back to its SQL type:

| Pattern                                                | Generator                 |
|--------------------------------------------------------|---------------------------|
| `email`, `*_email`                                     | realistic email address   |
| `name`, `first_name`, `last_name`, `display_name`, ... | person name               |
| `phone`, `tel`, `mobile`                               | phone number              |
| `*_url`, `link`, `homepage`, `avatar_url`              | URL                       |
| `address`, `city`, `country`, `zip`                    | postal address parts      |
| `description`, `bio`, `note`, `body`, `content`        | paragraph                 |
| `title`, `subject`, `headline`                         | sentence                  |
| `created_at`, `updated_at`, `*_at`                     | timestamp in past year    |
| `birthday`, `dob`                                      | past date                 |
| `age`                                                  | 0–100                     |
| `price`, `amount`, `cost`, `*_yen`                     | int in money range        |
| `count`, `quantity`, `qty`, `num_*`                    | int                       |
| `is_*`, `has_*`, `*_flag`, `enabled`                   | boolean                   |
| Postgres enum (`USER-DEFINED`)                         | random label              |
| anything else                                          | fallback by inferred Kind |

Name patterns above that produce text (names, addresses, prefectures, cities, phone numbers, postal codes) switch dictionaries when `--locale ja` is set; locale-neutral patterns like `email` and `*_url` keep their English forms.

`seeder` lets the database fill a column in exactly two cases:

- The column is `IDENTITY` (`id int GENERATED ALWAYS AS IDENTITY`).
- The column is **integer-typed and has a `DEFAULT`**. v0.1.0 does not
  parse the raw default expression, so any int with a default is left to
  the DB. In practice this covers `id serial` / `bigserial` (= `DEFAULT
  nextval(...)`), as well as plain counters like `version int DEFAULT 0`
  or `priority int DEFAULT -1`. The DB does whatever it was already going
  to do; `seeder` does not override it. (Parsing the default expression so
  only true `nextval(...)` columns are skipped is planned for v0.2.0.)

Every other column is generated by `seeder` — **even if it has a
`DEFAULT`**. So `created_at timestamptz DEFAULT now()` ends up with a
random past timestamp, not `now()`; `status text DEFAULT 'active'` ends up
with a fake word, not `'active'`. If you need the DB default for one of
these columns, exclude the table or accept the override.

> **Note on `json` / `jsonb` columns**: v0.1.0 emits randomly-structured
> placeholder JSON via `gofakeit.JSON(nil)`. Each value can be a multi-KB
> nested array/object, so seeding thousands of rows of `jsonb` is heavy on
> memory and bulk-insert throughput. `--exclude` the table or expect a
> slower run; per-column overrides are planned for v0.2.0.

> **Note on large `--rows`**: v0.1.0 generates every row in memory before
> handing the batch to `COPY`. Hundreds of thousands of rows are fine on
> a modern laptop, but `--rows 1,000,000+` can spike RAM into the GB range
> (more with `jsonb`). Streamed / chunked inserts are planned for v0.2.0.

### Foreign keys

Tables are inserted in dependency order: parents first, then children pick
a random parent PK for each FK column.

- **Self-FK** (e.g., `employees.manager_id REFERENCES employees(id)`):
  v0.1.0 builds the whole table in one batch, so the parent pool is empty
  the entire time. If `manager_id` is nullable, every row gets `NULL`. If
  it is `NOT NULL`, the constraint cannot be satisfied and `seeder`
  aborts. (Picking from already-generated rows is planned for v0.2.0.)
- **Real cycle** between distinct tables (`A → B → A`): there is no order
  that satisfies both directions, so `seeder` reports the cycle as an error
  rather than silently dropping one of the edges.

## Current scope

Postgres only. Single-column FKs only. English and Japanese locales. No
JSON/JSONB richer inference. Everything else — MySQL, more locales,
LLM-assisted text, polymorphic / composite FKs, alternate output modes,
existing-DB statistics sampling, raw `DEFAULT` parsing — remains on the
v0.2.0+ roadmap.

## Develop

```bash
make build              # ./bin/seeder
make test               # go test ./... -race
make lint               # golangci-lint run

# Bring up local Postgres + MySQL via docker compose
docker compose up -d

# Integration tests against the running Postgres
SEEDER_TEST_DSN=postgres://postgres:pass@localhost:5432/dev?sslmode=disable \
  make test-integration
```

## License

[MIT](./LICENSE).
