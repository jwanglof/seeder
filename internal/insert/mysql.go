package insert

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"github.com/mickamy/seeder/internal/dsn"
)

type mySQLDriver struct {
	db *sql.DB
}

func newMySQLDriver(ctx context.Context, dataSourceName string) (*mySQLDriver, error) {
	converted, err := dsn.ToMySQLDSN(dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("dsn: %w", err)
	}
	db, err := sql.Open("mysql", converted)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("ping: %w", err)
	}

	return &mySQLDriver{db: db}, nil
}

func (d *mySQLDriver) Close(_ context.Context) error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

// Truncate disables FK checks for the duration of the call. The session-scoped
// SET runs on a single pinned connection so the disable does not leak to
// other queries, and the deferred restore always runs even on error.
func (d *mySQLDriver) Truncate(ctx context.Context, tables []string) error {
	if len(tables) == 0 {
		return nil
	}

	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return fmt.Errorf("disable FK checks: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=1")
	}()

	for _, t := range tables {
		//nolint:gosec // G201: identifier is sanitized via quoteMySQLIdent (backtick-quoted)
		stmt := "TRUNCATE TABLE " + quoteMySQLIdent(t)
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}

	return nil
}

func (d *mySQLDriver) BulkInsert(ctx context.Context, table string, columns []string, rows [][]any) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	cols := make([]string, len(columns))
	for i, c := range columns {
		cols[i] = quoteMySQLIdent(c)
	}

	rowPh := "(" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
	rowsPh := strings.TrimSuffix(strings.Repeat(rowPh+",", len(rows)), ",")

	//nolint:gosec // G201: identifiers are sanitized via quoteMySQLIdent; values use placeholders
	stmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		quoteMySQLIdent(table),
		strings.Join(cols, ","),
		rowsPh,
	)

	args := make([]any, 0, len(rows)*len(columns))
	for _, row := range rows {
		args = append(args, row...)
	}

	if _, err := d.db.ExecContext(ctx, stmt, args...); err != nil {
		return 0, fmt.Errorf("exec: %w", err)
	}

	return int64(len(rows)), nil
}

func (d *mySQLDriver) ColumnValues(
	ctx context.Context,
	table string,
	columns []string,
) (map[string][]any, error) {
	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = quoteMySQLIdent(c)
	}
	//nolint:gosec // G201: identifiers are sanitized via quoteMySQLIdent
	stmt := fmt.Sprintf("SELECT %s FROM %s", strings.Join(quoted, ","), quoteMySQLIdent(table))

	rows, err := d.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string][]any, len(columns))
	for rows.Next() {
		dest := make([]any, len(columns))
		for i := range dest {
			var v any
			dest[i] = &v
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		for i, col := range columns {
			ptr, ok := dest[i].(*any)
			if !ok {
				return nil, fmt.Errorf("scan dest[%d] for %s is not *any", i, col)
			}
			out[col] = append(out[col], *ptr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return out, nil
}

func quoteMySQLIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
