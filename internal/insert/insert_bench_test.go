//go:build integration

package insert_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mickamy/seeder/internal/insert"
	"github.com/mickamy/seeder/internal/introspect"
	"github.com/mickamy/seeder/internal/plan"
)

// benchSchemaSQL is a small but representative schema: one parent (users),
// one child via NOT NULL FK (orders), and one grandchild touching two FKs
// (comments). seeder treats every column except identity / nextval as
// generator-driven, so the bench reflects realistic generator + FK pool work.
const benchSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE users (
    id         serial PRIMARY KEY,
    email      text   NOT NULL,
    name       text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id      serial PRIMARY KEY,
    user_id int    NOT NULL REFERENCES users(id),
    amount  int
);

CREATE TABLE comments (
    id       serial PRIMARY KEY,
    user_id  int    NOT NULL REFERENCES users(id),
    order_id int    REFERENCES orders(id),
    body     text
);
`

func benchSetup(b *testing.B) (string, introspect.Schema, []string) {
	b.Helper()
	// SEEDER_BENCH_DSN is intentionally separate from
	// SEEDER_TEST_DSN_POSTGRES: this benchmark runs
	// `DROP SCHEMA public CASCADE`, so the DSN must point at a disposable
	// database (typically the compose Postgres). Requiring an explicit
	// opt-in env var avoids accidentally wiping a shared dev / CI database
	// whose DSN happens to be exported for integration tests.
	dsn := os.Getenv("SEEDER_BENCH_DSN")
	if dsn == "" {
		b.Skip("SEEDER_BENCH_DSN not set (benchmarks drop the public schema; set it to a disposable Postgres DSN)")
	}

	ctx := b.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, benchSchemaSQL); err != nil {
		b.Fatalf("schema: %v", err)
	}
	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		b.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		b.Fatalf("plan: %v", err)
	}

	return dsn, schema, order
}

// usersOnly returns a schema/order pair restricted to the users table. Used
// to measure raw generator + COPY throughput without FK pool overhead.
func usersOnly(b *testing.B, schema introspect.Schema) (introspect.Schema, []string) {
	b.Helper()
	for _, t := range schema.Tables {
		if t.Name == "users" {
			return introspect.Schema{Tables: []introspect.Table{t}}, []string{"users"}
		}
	}
	b.Fatalf("users table missing from schema")

	return introspect.Schema{}, nil
}

func BenchmarkInsert_SingleTable_100k(b *testing.B) {
	dsn, schema, _ := benchSetup(b)
	schema, order := usersOnly(b, schema)
	opts := insert.Options{
		Rows: 100_000, BatchSize: 1000,
		Seed: new(uint64(42)), Truncate: true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := insert.Run(b.Context(), dsn, schema, order, opts, io.Discard); err != nil {
			b.Fatalf("insert.Run: %v", err)
		}
	}
}

func BenchmarkInsert_FKChain_50k(b *testing.B) {
	dsn, schema, order := benchSetup(b)
	opts := insert.Options{
		Rows: 50_000, BatchSize: 1000,
		Seed: new(uint64(42)), Truncate: true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := insert.Run(b.Context(), dsn, schema, order, opts, io.Discard); err != nil {
			b.Fatalf("insert.Run: %v", err)
		}
	}
}

// BenchmarkInsert_BatchSize sweeps --batch-size against a fixed 20k FK chain
// to show how the in-memory batch / DB round-trip ratio shifts.
func BenchmarkInsert_BatchSize(b *testing.B) {
	dsn, schema, order := benchSetup(b)

	for _, bs := range []int{1, 100, 1000, 10_000} {
		b.Run(fmt.Sprintf("batch=%d", bs), func(b *testing.B) {
			opts := insert.Options{
				Rows: 20_000, BatchSize: bs,
				Seed: new(uint64(42)), Truncate: true,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := insert.Run(b.Context(), dsn, schema, order, opts, io.Discard); err != nil {
					b.Fatalf("insert.Run: %v", err)
				}
			}
		})
	}
}
