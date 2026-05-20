//go:build integration

package insert_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/mickamy/seeder/internal/dsn"
	"github.com/mickamy/seeder/internal/insert"
	"github.com/mickamy/seeder/internal/introspect"
	"github.com/mickamy/seeder/internal/plan"
	"github.com/mickamy/seeder/internal/tsql"
)

const mysqlInsertSchemaSQL = `
CREATE TABLE users (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    email      VARCHAR(255) NOT NULL,
    name       VARCHAR(255),
    bio        TEXT,
    is_active  TINYINT(1)   NOT NULL DEFAULT 1,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE orders (
    id      INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    status  ENUM('pending','paid','shipped') NOT NULL,
    amount  INT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE comments (
    id       INT AUTO_INCREMENT PRIMARY KEY,
    user_id  INT NOT NULL,
    order_id INT,
    body     TEXT,
    FOREIGN KEY (user_id)  REFERENCES users(id),
    FOREIGN KEY (order_id) REFERENCES orders(id)
)
`

//nolint:paralleltest,tparallel // mutates schema; cannot run in parallel
func TestRun_MySQL_Basic(t *testing.T) {
	rawDSN := os.Getenv("SEEDER_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SEEDER_TEST_DSN_MYSQL not set")
	}

	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("ToMySQLDSN: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	if err := applyMySQLInsertSchema(ctx, db, mysqlInsertSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var buf bytes.Buffer
	seed := uint64(42)
	stats, err := insert.Run(ctx, rawDSN, schema, order, insert.Options{
		Rows: 25,
		Seed: &seed,
	}, &buf)
	if err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	wantTables := map[string]bool{"users": true, "orders": true, "comments": true}
	for _, s := range stats {
		if !wantTables[s.Table] {
			t.Errorf("unexpected table in stats: %s", s.Table)
		}
		if s.Rows != 25 {
			t.Errorf("%s rows = %d; want 25", s.Table, s.Rows)
		}
	}

	for table := range wantTables {
		var n int
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`")
		if err := row.Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 25 {
			t.Errorf("%s count = %d; want 25", table, n)
		}
	}
}

const mysqlDefaultsSchemaSQL = `
CREATE TABLE counters (
    id    INT AUTO_INCREMENT PRIMARY KEY,
    label VARCHAR(255) NOT NULL,
    score INT          NOT NULL DEFAULT 0
)
`

//nolint:paralleltest,tparallel // mutates schema; cannot run in parallel
func TestRun_MySQL_DefaultColumnsAreOverridden(t *testing.T) {
	rawDSN := os.Getenv("SEEDER_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SEEDER_TEST_DSN_MYSQL not set")
	}

	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("ToMySQLDSN: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	if err := applyMySQLInsertSchema(ctx, db, mysqlDefaultsSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	seed := uint64(42)
	var buf bytes.Buffer
	if _, err := insert.Run(ctx, rawDSN, schema, order, insert.Options{Rows: 50, Seed: &seed}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var total, zeroScores int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*), SUM(score = 0) FROM counters").Scan(&total, &zeroScores); err != nil {
		t.Fatalf("counters check: %v", err)
	}
	if total != 50 {
		t.Errorf("counters total = %d; want 50", total)
	}
	if zeroScores == total {
		t.Errorf("every counters row has score=0; DEFAULT 0 was not overridden")
	}
}

const mysqlUniqueSchemaSQL = `
CREATE TABLE tags (
    id   INT AUTO_INCREMENT PRIMARY KEY,
    slug VARCHAR(255) NOT NULL UNIQUE
)
`

//nolint:paralleltest,tparallel // mutates schema; cannot run in parallel
func TestRun_MySQL_UniqueColumnsAreDistinct(t *testing.T) {
	rawDSN := os.Getenv("SEEDER_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SEEDER_TEST_DSN_MYSQL not set")
	}

	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("ToMySQLDSN: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	if err := applyMySQLInsertSchema(ctx, db, mysqlUniqueSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	seed := uint64(42)
	var buf bytes.Buffer
	if _, err := insert.Run(ctx, rawDSN, schema, order, insert.Options{Rows: 50, Seed: &seed}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var distinctSlugs int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT slug) FROM tags").Scan(&distinctSlugs); err != nil {
		t.Fatalf("tags distinct check: %v", err)
	}
	if distinctSlugs != 50 {
		t.Errorf("tags distinct slugs = %d; want 50 (UNIQUE-aware generator collided)", distinctSlugs)
	}
}

const mysqlCompositeFKSchemaSQL = `
CREATE TABLE projects (
    id     INT AUTO_INCREMENT PRIMARY KEY,
    code   VARCHAR(255) NOT NULL UNIQUE,
    region VARCHAR(255) NOT NULL,
    name   VARCHAR(255) NOT NULL,
    UNIQUE (code, region)
);

CREATE TABLE tasks (
    id             INT AUTO_INCREMENT PRIMARY KEY,
    project_code   VARCHAR(255) NOT NULL,
    project_region VARCHAR(255) NOT NULL,
    title          VARCHAR(255) NOT NULL,
    FOREIGN KEY (project_code, project_region) REFERENCES projects(code, region)
)
`

//nolint:paralleltest,tparallel // mutates schema; cannot run in parallel
func TestRun_MySQL_CompositeFK(t *testing.T) {
	rawDSN := os.Getenv("SEEDER_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SEEDER_TEST_DSN_MYSQL not set")
	}

	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("ToMySQLDSN: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	if err := applyMySQLInsertSchema(ctx, db, mysqlCompositeFKSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	seed := uint64(42)
	var buf bytes.Buffer
	if _, err := insert.Run(ctx, rawDSN, schema, order, insert.Options{Rows: 30, Seed: &seed}, &buf); err != nil {
		t.Fatalf("insert.Run: %v", err)
	}

	var orphans int
	if err := db.QueryRowContext(ctx, `
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

	var mismatches int
	if err := db.QueryRowContext(ctx, `
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

//nolint:paralleltest,tparallel // mutates schema; cannot run in parallel
func TestRun_MySQL_Truncate(t *testing.T) {
	rawDSN := os.Getenv("SEEDER_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SEEDER_TEST_DSN_MYSQL not set")
	}

	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("ToMySQLDSN: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	if err := applyMySQLInsertSchema(ctx, db, mysqlInsertSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	order, err := plan.Build(schema.Tables)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	seed := uint64(42)
	var buf bytes.Buffer
	if _, err := insert.Run(ctx, rawDSN, schema, order, insert.Options{Rows: 10, Seed: &seed}, &buf); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if _, err := insert.Run(ctx, rawDSN, schema, order, insert.Options{Rows: 10, Seed: &seed, Truncate: true}, &buf); err != nil {
		t.Fatalf("truncate run: %v", err)
	}

	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `users`").Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if n != 10 {
		t.Errorf("after truncate+insert users count = %d; want 10 (FK checks disabled during truncate)", n)
	}
}

func applyMySQLInsertSchema(ctx context.Context, db *sql.DB, schemaSQL string) error {
	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		return err
	}
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
