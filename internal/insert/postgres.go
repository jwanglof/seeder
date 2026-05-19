package insert

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type postgresDriver struct {
	conn *pgx.Conn
}

func newPostgresDriver(ctx context.Context, dataSourceName string) (*postgresDriver, error) {
	conn, err := pgx.Connect(ctx, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return &postgresDriver{conn: conn}, nil
}

func (d *postgresDriver) Close(ctx context.Context) error {
	if err := d.conn.Close(ctx); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

func (d *postgresDriver) Truncate(ctx context.Context, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		parts = append(parts, pgx.Identifier{t}.Sanitize())
	}
	stmt := fmt.Sprintf("TRUNCATE %s RESTART IDENTITY CASCADE", strings.Join(parts, ", "))
	if _, err := d.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}

	return nil
}

func (d *postgresDriver) BulkInsert(ctx context.Context, table string, columns []string, rows [][]any) (int64, error) {
	n, err := d.conn.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return 0, fmt.Errorf("CopyFrom: %w", err)
	}

	return n, nil
}

func (d *postgresDriver) ColumnValues(
	ctx context.Context,
	table string,
	columns []string,
) (map[string][]any, error) {
	quoted := make([]string, len(columns))
	for i, col := range columns {
		quoted[i] = pgx.Identifier{col}.Sanitize()
	}
	stmt := fmt.Sprintf("SELECT %s FROM %s", strings.Join(quoted, ", "), pgx.Identifier{table}.Sanitize())

	rows, err := d.conn.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]any, len(columns))
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("values: %w", err)
		}
		for i, col := range columns {
			out[col] = append(out[col], vals[i])
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return out, nil
}
