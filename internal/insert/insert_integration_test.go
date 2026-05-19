//go:build integration

package insert_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mickamy/seeder/internal/insert"
	"github.com/mickamy/seeder/internal/introspect"
	"github.com/mickamy/seeder/internal/plan"
)

const schemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped');

CREATE TABLE users (
    id         serial      PRIMARY KEY,
    email      text        NOT NULL,
    name       text,
    bio        text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id       serial       PRIMARY KEY,
    user_id  int          NOT NULL REFERENCES users(id),
    status   order_status NOT NULL,
    amount   int
);

CREATE TABLE comments (
    id       serial PRIMARY KEY,
    user_id  int    NOT NULL REFERENCES users(id),
    order_id int    REFERENCES orders(id),
    body     text
);

CREATE TABLE accounts (
    id   serial PRIMARY KEY,
    uid  uuid   NOT NULL UNIQUE,
    name text
);

CREATE TABLE memberships (
    id          serial PRIMARY KEY,
    account_uid uuid   NOT NULL REFERENCES accounts(uid),
    role        text
);

CREATE TABLE blobs (
    id      serial PRIMARY KEY,
    payload bytea  NOT NULL
);
`

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_Basic(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var buf bytes.Buffer
	stats, err := insert.Run(ctx, dsn, schema, order, insert.Options{
		Rows: 50,
		Seed: new(uint64(42)),
	}, &buf)
	if err != nil {
		t.Fatalf("insert.Run: %v", err)
	}
	if len(stats) != 6 {
		t.Fatalf("stats len = %d; want 6", len(stats))
	}
	for _, s := range stats {
		if s.Rows != 50 {
			t.Errorf("%s rows = %d; want 50", s.Table, s.Rows)
		}
	}

	for _, table := range []string{"users", "orders", "comments", "accounts", "memberships", "blobs"} {
		var count int
		row := conn.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", pgx.Identifier{table}.Sanitize()))
		if err := row.Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 50 {
			t.Errorf("%s count = %d; want 50", table, count)
		}
	}

	var orphans int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM orders WHERE user_id NOT IN (SELECT id FROM users)").Scan(&orphans); err != nil {
		t.Fatalf("orders FK check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("orders with missing user_id = %d; want 0", orphans)
	}

	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM comments WHERE user_id NOT IN (SELECT id FROM users)").Scan(&orphans); err != nil {
		t.Fatalf("comments user_id FK check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("comments with missing user_id = %d; want 0", orphans)
	}

	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM comments WHERE order_id IS NOT NULL AND order_id NOT IN (SELECT id FROM orders)").Scan(&orphans); err != nil {
		t.Fatalf("comments order_id FK check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("comments with missing order_id = %d; want 0", orphans)
	}

	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM memberships WHERE account_uid NOT IN (SELECT uid FROM accounts)").Scan(&orphans); err != nil {
		t.Fatalf("memberships account_uid FK check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("memberships pointing at missing account_uid (UNIQUE non-PK FK) = %d; want 0", orphans)
	}

	var emptyBlobs int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM blobs WHERE payload IS NULL OR octet_length(payload) = 0").Scan(&emptyBlobs); err != nil {
		t.Fatalf("blobs payload check: %v", err)
	}
	if emptyBlobs != 0 {
		t.Errorf("blobs with NULL or empty bytea payload = %d; want 0", emptyBlobs)
	}

	var bad int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM orders WHERE status NOT IN ('pending','paid','shipped')").Scan(&bad); err != nil {
		t.Fatalf("status check: %v", err)
	}
	if bad != 0 {
		t.Errorf("orders with invalid status = %d", bad)
	}

	var nullEmails int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE email IS NULL OR email = ''").Scan(&nullEmails); err != nil {
		t.Fatalf("email check: %v", err)
	}
	if nullEmails != 0 {
		t.Errorf("users with NULL/empty email = %d", nullEmails)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_Determinism(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	apply := func() []string {
		t.Helper()
		if _, err := conn.Exec(ctx, schemaSQL); err != nil {
			t.Fatalf("apply schema: %v", err)
		}
		schema, err := introspect.Do(ctx, dsn)
		if err != nil {
			t.Fatalf("introspect: %v", err)
		}
		order, err := plan.Build(schema.Tables)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		var buf bytes.Buffer
		if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 10, Seed: new(uint64(42))}, &buf); err != nil {
			t.Fatalf("insert.Run: %v", err)
		}
		rows, err := conn.Query(ctx, "SELECT email FROM users ORDER BY id")
		if err != nil {
			t.Fatalf("select emails: %v", err)
		}
		defer rows.Close()
		var emails []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			emails = append(emails, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		return emails
	}

	first := apply()
	second := apply()
	if len(first) != len(second) {
		t.Fatalf("len mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("row %d: %q vs %q (seed=42 should be deterministic)", i, first[i], second[i])
		}
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_DryRun(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var buf bytes.Buffer
	if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 10, Seed: new(uint64(42)), DryRun: true}, &buf); err != nil {
		t.Fatalf("insert.Run dry-run: %v", err)
	}

	var count int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("dry-run inserted rows: users count = %d; want 0", count)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_Truncate(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO users (email) VALUES ('preexisting@example.com')"); err != nil {
		t.Fatalf("insert preexisting: %v", err)
	}

	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var buf bytes.Buffer
	if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 20, Seed: new(uint64(42)), Truncate: true}, &buf); err != nil {
		t.Fatalf("insert.Run truncate: %v", err)
	}

	var preexisting int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE email = 'preexisting@example.com'").Scan(&preexisting); err != nil {
		t.Fatalf("preexisting check: %v", err)
	}
	if preexisting != 0 {
		t.Errorf("preexisting row survived TRUNCATE: %d", preexisting)
	}

	var total int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&total); err != nil {
		t.Fatalf("total check: %v", err)
	}
	if total != 20 {
		t.Errorf("users total = %d; want 20", total)
	}
}
