# Insert benchmarks

Measures raw `insert.Run` throughput against a compose Postgres. Numbers come
from:

```bash
docker compose up -d postgres
SEEDER_TEST_DSN_POSTGRES='postgres://postgres:pass@localhost:5432/dev?sslmode=disable' \
  go test -tags=integration -bench=. -benchmem -benchtime=5x -run=^$ \
    ./internal/insert/...
```

## Setup

- Host: Apple M4 Max (arm64), macOS
- Postgres 16 from `compose.yaml` (`docker compose up -d postgres`)
- Schema: `users` (serial PK) → `orders` (NOT NULL FK) → `comments` (one NOT
  NULL + one nullable FK). See `internal/insert/insert_bench_test.go::benchSchemaSQL`.
- Seed: fixed `42` so timing variance is generator / runtime only.

## Results (2026-05-21)

| Benchmark                 |           Rows | Batch | time/op | MB/op | allocs/op |
|---------------------------|---------------:|------:|--------:|------:|----------:|
| `Insert_SingleTable_100k` | 100k (`users`) |  1000 |  419 ms |    91 |     1.21M |
| `Insert_FKChain_50k`      | 50k × 3 tables |  1000 |  1.28 s |   246 |     2.13M |
| `BatchSize=1`             | 20k × 3 tables |     1 | 14.41 s |   200 |     2.95M |
| `BatchSize=100`           | 20k × 3 tables |   100 |  686 ms |   133 |      875k |
| `BatchSize=1000`          | 20k × 3 tables |  1000 |  511 ms |    98 |      852k |
| `BatchSize=10000`         | 20k × 3 tables | 10000 |  367 ms |    87 |      850k |

## Observations

- **`--batch-size 1` is ~28× slower** than `--batch-size 1000` on the 20k FK
  chain. Per-row COPY round-trips dominate; the default 1000 is sized so the
  COPY payload amortizes the round-trip cost without holding too many rows
  in memory at once.
- Moving from 1000 → 10000 buys another ~28% wall-time and trims ~11 MB off
  the in-memory peak, but both the FK pool and the in-batch self-FK buffer
  are capped (`defaultPoolCapacity = 100_000`), so larger batches do not
  blow up memory; the gain is mostly fewer Go-side allocations.
- 100k single-table inserts complete in about 0.42 s (~240k rows/sec). The
  3-table FK chain at 50k each (150k rows total) costs ~1.28 s (~117k rows/sec);
  the extra time tracks closely with the `Driver.ColumnValues` refresh that
  refills the FK pool after each parent table.

## Notes for reading these numbers

- `time/op` is wall time per `b.N` iteration; with `-benchtime=5x` every
  benchmark runs five full `insert.Run` cycles (`Truncate: true`, fixed
  seed), so `time/op` already includes the `TRUNCATE`.
- `MB/op` aggregates every allocation during the run, not the resident
  set. Steady-state memory is bounded by `--batch-size` × per-row tuple
  size plus the 100k-cap FK pool, so peak RSS is well below `MB/op`.
- Stream / CDC mode is not in this sweep yet; its per-tick cost is
  dominated by the rate setting and is excluded so the comparison stays
  apples-to-apples.

## Reproducing

```bash
docker compose up -d postgres
SEEDER_TEST_DSN_POSTGRES='postgres://postgres:pass@localhost:5432/dev?sslmode=disable' \
  go test -tags=integration -bench=. -benchmem -benchtime=5x -run=^$ \
    ./internal/insert/...
```

Bump to `-benchtime=10x` (or longer) for tighter numbers. The 5x sweep above
finishes in about 107 s on the host listed in Setup.
