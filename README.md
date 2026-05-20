# seeder

> Zero-config database seeder — one command, realistic data, no factory code.

`seeder` populates your MySQL or Postgres database with realistic fake data
straight from the schema. No factory code, no YAML, no AI key. Point it at a
DSN and it figures out the rest: it introspects your tables, infers what each
column should look like from its name and type, and bulk-inserts (multi-row
`INSERT` on MySQL, `COPY` on Postgres) while respecting your foreign-key
constraints.

```bash
$ seeder mysql://root:pass@localhost:3306/mydb --rows 1000 --truncate --seed 42
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
  --batch-size int Rows generated per INSERT batch (default: 1000)
  --cache <file>   Load/save introspected schema to this file (delete to invalidate)
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

**Large schemas / repeated runs.** Cache the introspected schema so subsequent runs skip the `information_schema` round-trip. Delete the file after a migration to invalidate.

```bash
seeder $DATABASE_URL --cache /tmp/seeder-schema.gob --rows 5000
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

- `generator: <Name>` — force a built-in generator (e.g., `Email`, `UUID`, `Phone`, `PastDate`). Supplying an unknown name surfaces the full known list as part of the preflight error. Built-ins resolve via gofakeit defaults and do not switch on `--locale`; use `value:` for a fixed string when you need a specific locale.
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

`seeder` queries `information_schema` (plus `pg_catalog` / `pg_constraint` on
Postgres for enum types and composite-FK column ordering) for tables, columns,
primary keys, foreign keys, and enum labels. The current database is scoped
via `DATABASE()` on MySQL; the `public` schema is scoped on Postgres. No
schema changes, no privileged access — just standard SELECTs.

### Smart inference

Each column is matched against a small set of name patterns first, then
falls back to its SQL type:

| Pattern                                                | Generator                 |
|--------------------------------------------------------|---------------------------|
| `email`, `*_email`                                     | realistic email address   |
| `name`, `first_name`, `last_name`, `display_name`, ... | person name               |
| `phone`, `tel`, `mobile`                               | phone number              |
| `*_url`, `link`, `homepage`, `website`                 | URL                       |
| `avatar`, `image`, `photo`, `picture`, `thumbnail` (also `_url`-suffixed) | image placeholder URL |
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

- The column is `IDENTITY` — Postgres `GENERATED ALWAYS AS IDENTITY` or
  MySQL `AUTO_INCREMENT`.
- The column's default is a Postgres sequence call (`DEFAULT
  nextval('...')`). This is the form `serial` / `bigserial` expand into;
  `seeder` leaves it to the DB so the sequence stays authoritative.

Every other column is generated by `seeder` — **even if it has a
`DEFAULT`**. Plain defaults like `score int NOT NULL DEFAULT 0`,
`created_at timestamptz DEFAULT now()`, or `status text DEFAULT 'active'`
are all overridden with seeder-generated values so test data is varied
instead of every row sharing the same literal. If you need the DB default
for one of these columns, exclude the table.

Columns covered by a single-column `UNIQUE` constraint are detected during
introspection and routed to a collision-avoiding generator: `email`
becomes `<uuid>@example.com`, other string columns get a UUID suffix on
top of the matched name rule, and integer columns are widened to a much
larger range. Composite `UNIQUE` constraints are not flagged because no
single per-column generator can guarantee combined uniqueness.

> **Note on `json` / `jsonb` columns**: v0.1.0 emits randomly-structured
> placeholder JSON via `gofakeit.JSON(nil)`. Each value can be a multi-KB
> nested array/object, so seeding thousands of rows of `jsonb` is heavy on
> memory and bulk-insert throughput. `--exclude` the table or expect a
> slower run; per-column overrides are planned for v0.2.0.

> **Note on large `--rows`**: `seeder` generates rows in chunks of
> `--batch-size` (default 1000) and flushes each chunk via `COPY` / multi-row
> `INSERT` before building the next, so memory stays bounded even at
> millions of rows. The parent-PK pool used for FK resolution is also capped
> at 100k values per (table, column); when refilled it keeps the tail of
> what the driver returned (i.e., only the last 100k entries are kept). The
> in-batch self-FK buffer used for forward-references shares the same 100k
> cap, so self-referential tables with seeder-generated PKs do not grow
> unbounded either.

### Foreign keys

Tables are inserted in dependency order: parents first, then children pick
a random parent PK for each FK column.

- **Self-FK** (e.g., `employees.manager_id REFERENCES employees(id)`):
  `seeder` resolves these forward-references inside the batch. Each row's
  self-FK can point at a PK already generated earlier in the same batch;
  the very first row picks its own PK (self-loop) when the constraint is
  `NOT NULL`. This only works when the referenced PK is seeder-generated
  (e.g., `uuid` PK on Postgres). DB-generated PKs such as `serial` or
  `AUTO_INCREMENT` are unknown until after the insert, so a `NOT NULL`
  self-FK against them still errors out; a nullable self-FK against them
  silently sets `NULL` on every row.
- **Real cycle** between distinct tables (`A → B → A`): there is no order
  that satisfies both directions, so `seeder` reports the cycle as an error
  rather than silently dropping one of the edges.

## Current scope

Single-column FKs only. English and Japanese locales. No JSON/JSONB richer
inference. Everything else — more locales, LLM-assisted text, polymorphic /
composite FKs, alternate output modes, existing-DB statistics sampling —
remains on the roadmap.

## Develop

```bash
make build              # ./bin/seeder
make test               # go test ./... -race
make lint               # golangci-lint run

# Bring up local Postgres + MySQL via docker compose
docker compose up -d

# Integration tests (set either or both; unset drivers skip their tests)
SEEDER_TEST_DSN_MYSQL=mysql://root:pass@localhost:3306/dev?parseTime=true \
SEEDER_TEST_DSN_POSTGRES=postgres://postgres:pass@localhost:5432/dev?sslmode=disable \
  make test-integration
```

## License

[MIT](./LICENSE).
