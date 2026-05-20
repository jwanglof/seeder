//go:build integration

package insert_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

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
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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

const defaultsSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE counters (
    id    serial PRIMARY KEY,
    label text   NOT NULL,
    score int    NOT NULL DEFAULT 0
);
`

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_DefaultColumnsAreOverridden(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, defaultsSchemaSQL); err != nil {
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
	if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 50, Seed: new(uint64(42))}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var total, zeroScores int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*), COUNT(*) FILTER (WHERE score = 0) FROM counters").Scan(&total, &zeroScores); err != nil {
		t.Fatalf("counters check: %v", err)
	}
	if total != 50 {
		t.Errorf("counters total = %d; want 50", total)
	}
	if zeroScores == total {
		t.Errorf("every counters row has score=0; DEFAULT 0 was not overridden")
	}
}

const uniqueSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE tags (
    id   serial PRIMARY KEY,
    slug text   NOT NULL UNIQUE
);
`

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_UniqueColumnsAreDistinct(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, uniqueSchemaSQL); err != nil {
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
	if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 50, Seed: new(uint64(42))}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var distinctSlugs int
	if err := conn.QueryRow(ctx, "SELECT COUNT(DISTINCT slug) FROM tags").Scan(&distinctSlugs); err != nil {
		t.Fatalf("tags distinct check: %v", err)
	}
	if distinctSlugs != 50 {
		t.Errorf("tags distinct slugs = %d; want 50 (UNIQUE-aware generator collided)", distinctSlugs)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_BatchSizeIsDeterministic(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	runWith := func(batchSize int) []string {
		t.Helper()
		if _, err := conn.Exec(ctx, uniqueSchemaSQL); err != nil {
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
		if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{
			Rows:      20,
			BatchSize: batchSize,
			Seed:      new(uint64(42)),
		}, &buf); err != nil {
			t.Fatalf("insert.Run batch=%d: %v", batchSize, err)
		}
		rows, err := conn.Query(ctx, "SELECT slug FROM tags ORDER BY id")
		if err != nil {
			t.Fatalf("select slugs: %v", err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		return out
	}

	single := runWith(1)
	multi := runWith(10)
	if len(single) != len(multi) {
		t.Fatalf("len mismatch: %d vs %d", len(single), len(multi))
	}
	for i := range single {
		if single[i] != multi[i] {
			t.Errorf("row %d: %q vs %q (batch-size must not affect output for seeded RNG)", i, single[i], multi[i])
		}
	}
}

const selfFKSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE tree_nodes (
    id        uuid PRIMARY KEY,
    parent_id uuid REFERENCES tree_nodes(id),
    name      text NOT NULL
);

CREATE TABLE forest_nodes (
    id        uuid PRIMARY KEY,
    parent_id uuid NOT NULL REFERENCES forest_nodes(id),
    name      text NOT NULL
);
`

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_SelfFKForwardReference(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, selfFKSchemaSQL); err != nil {
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
	if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 50, Seed: new(uint64(42))}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var treeNonNullParents int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM tree_nodes WHERE parent_id IS NOT NULL").Scan(&treeNonNullParents); err != nil {
		t.Fatalf("tree_nodes parent_id check: %v", err)
	}
	if treeNonNullParents == 0 {
		t.Errorf("tree_nodes: every parent_id is NULL; nullable self-FK forward-reference is broken")
	}

	var treeOrphans int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM tree_nodes WHERE parent_id IS NOT NULL AND parent_id NOT IN (SELECT id FROM tree_nodes)").Scan(&treeOrphans); err != nil {
		t.Fatalf("tree_nodes orphan check: %v", err)
	}
	if treeOrphans != 0 {
		t.Errorf("tree_nodes orphan parent_id = %d", treeOrphans)
	}

	var forestNullParents int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM forest_nodes WHERE parent_id IS NULL").Scan(&forestNullParents); err != nil {
		t.Fatalf("forest_nodes NULL check: %v", err)
	}
	if forestNullParents != 0 {
		t.Errorf("forest_nodes NULL parent_id = %d; NOT NULL self-FK should always resolve", forestNullParents)
	}

	var forestOrphans int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM forest_nodes WHERE parent_id NOT IN (SELECT id FROM forest_nodes)").Scan(&forestOrphans); err != nil {
		t.Fatalf("forest_nodes orphan check: %v", err)
	}
	if forestOrphans != 0 {
		t.Errorf("forest_nodes orphan parent_id = %d", forestOrphans)
	}
}

const compositeFKSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE projects (
    id     serial PRIMARY KEY,
    code   text   NOT NULL UNIQUE,
    region text   NOT NULL,
    name   text   NOT NULL,
    UNIQUE (code, region)
);

CREATE TABLE tasks (
    id             serial PRIMARY KEY,
    project_code   text   NOT NULL,
    project_region text   NOT NULL,
    title          text   NOT NULL,
    FOREIGN KEY (project_code, project_region) REFERENCES projects (code, region)
);
`

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_CompositeFK(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, compositeFKSchemaSQL); err != nil {
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
	if _, err := insert.Run(ctx, dsn, schema, order, insert.Options{Rows: 30, Seed: new(uint64(42))}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var orphans int
	if err := conn.QueryRow(ctx, `
        SELECT COUNT(*) FROM tasks t
        WHERE NOT EXISTS (
            SELECT 1 FROM projects p
            WHERE p.code = t.project_code AND p.region = t.project_region
        )
    `).Scan(&orphans); err != nil {
		t.Fatalf("composite FK orphan check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("tasks with no matching (code, region) in projects = %d; want 0", orphans)
	}

	// Confirm the FK group resolved as a tuple, not by independent picks
	// per column — otherwise (code, region) pairs that never coexist in
	// projects can appear in tasks.
	var mismatches int
	if err := conn.QueryRow(ctx, `
        SELECT COUNT(*) FROM tasks t
        JOIN projects p ON p.code = t.project_code
        WHERE p.region <> t.project_region
    `).Scan(&mismatches); err != nil {
		t.Fatalf("tuple integrity check: %v", err)
	}
	if mismatches != 0 {
		t.Errorf("tasks where project_code matches but project_region does not = %d; want 0", mismatches)
	}
}

const polymorphicSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE posts (
    id    serial PRIMARY KEY,
    title text NOT NULL
);

CREATE TABLE articles (
    id    serial PRIMARY KEY,
    title text NOT NULL
);

CREATE TABLE comments (
    id               serial PRIMARY KEY,
    commentable_type text NOT NULL,
    commentable_id   int  NOT NULL,
    body             text NOT NULL
);
`

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_PolymorphicFK(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, polymorphicSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.BuildWithDeps(schema.Tables, map[string][]string{
		"comments": {"posts", "articles"},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	opts := insert.Options{
		Rows: 30,
		Seed: new(uint64(42)),
		Polymorphic: map[string][]insert.PolymorphicSpec{
			"comments": {
				{
					TypeColumn: "commentable_type",
					IDColumn:   "commentable_id",
					Targets: []insert.PolymorphicTarget{
						{Table: "posts", Type: "Post", IDCol: "id"},
						{Table: "articles", Type: "Article", IDCol: "id"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if _, err := insert.Run(ctx, dsn, schema, order, opts, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	for _, typ := range []string{"Post", "Article"} {
		var n int
		if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM comments WHERE commentable_type = $1", typ).Scan(&n); err != nil {
			t.Fatalf("count comments by type %s: %v", typ, err)
		}
		if n == 0 {
			t.Errorf("no comments resolved to %s; type discriminator distribution looks degenerate", typ)
		}
	}

	var orphans int
	if err := conn.QueryRow(ctx, `
        SELECT COUNT(*) FROM comments c
        WHERE (c.commentable_type = 'Post'    AND NOT EXISTS (SELECT 1 FROM posts    p WHERE p.id = c.commentable_id))
           OR (c.commentable_type = 'Article' AND NOT EXISTS (SELECT 1 FROM articles a WHERE a.id = c.commentable_id))
    `).Scan(&orphans); err != nil {
		t.Fatalf("polymorphic orphan check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("comments pointing at missing target = %d; want 0", orphans)
	}

	var unknown int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM comments WHERE commentable_type NOT IN ('Post','Article')").Scan(&unknown); err != nil {
		t.Fatalf("unknown type check: %v", err)
	}
	if unknown != 0 {
		t.Errorf("comments with unknown commentable_type = %d", unknown)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_OutputSQL(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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

	var sqlBuf bytes.Buffer
	opts := insert.Options{
		Rows:         5,
		Seed:         new(uint64(42)),
		OutputMode:   "sql",
		OutputWriter: &sqlBuf,
	}
	if _, err := insert.Run(ctx, dsn, schema, order, opts, io.Discard); err != nil {
		t.Fatalf("insert.Run output=sql: %v", err)
	}

	out := sqlBuf.String()
	for _, table := range []string{"users", "orders", "comments"} {
		if !strings.Contains(out, "INSERT INTO \""+table+"\"") {
			t.Errorf("output missing INSERT for %s; got:\n%s", table, out)
		}
	}

	var rowCount int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&rowCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("--output=sql should not write to DB; users count = %d; want 0", rowCount)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_OutputNDJSON(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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

	var ndBuf bytes.Buffer
	opts := insert.Options{
		Rows:         5,
		Seed:         new(uint64(42)),
		OutputMode:   "ndjson",
		OutputWriter: &ndBuf,
	}
	if _, err := insert.Run(ctx, dsn, schema, order, opts, io.Discard); err != nil {
		t.Fatalf("insert.Run output=ndjson: %v", err)
	}

	lines := strings.Split(strings.TrimRight(ndBuf.String(), "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("ndjson lines = %d; want at least 5", len(lines))
	}
	seenTables := make(map[string]bool)
	for _, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("invalid ndjson line %q: %v", line, err)
		}
		tableVal, ok := obj["_table"].(string)
		if !ok || tableVal == "" {
			t.Errorf("ndjson line missing _table: %s", line)
		}
		seenTables[tableVal] = true
	}
	for _, table := range []string{"users", "orders"} {
		if !seenTables[table] {
			t.Errorf("ndjson missing %s table emission", table)
		}
	}

	var rowCount int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&rowCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("--output=ndjson should not write to DB; users count = %d; want 0", rowCount)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRunStream(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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

	streamCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	err = insert.RunStream(streamCtx, dsn, schema, order, insert.Options{
		Rows: 5,
		Seed: new(uint64(42)),
	}, insert.StreamOptions{Rate: 30}, &buf)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunStream: %v", err)
	}

	var totalUsers int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&totalUsers); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if totalUsers <= 5 {
		t.Errorf("RunStream appended no rows: users = %d; want > 5 (initial seed)", totalUsers)
	}
}

//nolint:paralleltest,tparallel // mutates the public schema
func TestRun_Truncate(t *testing.T) {
	dsn := os.Getenv("SEEDER_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SEEDER_TEST_DSN_POSTGRES not set")
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
