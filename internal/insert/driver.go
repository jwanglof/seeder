package insert

import (
	"context"
	"fmt"

	"github.com/mickamy/seeder/internal/dsn"
)

type Driver interface {
	Close(ctx context.Context) error
	Truncate(ctx context.Context, tables []string) error
	BulkInsert(ctx context.Context, table string, columns []string, rows [][]any) (int64, error)
	ColumnValues(ctx context.Context, table string, columns []string) (map[string][]any, error)
}

func openDriver(ctx context.Context, dataSourceName string) (Driver, error) {
	switch dsn.Scheme(dataSourceName) {
	case "mysql":
		return newMySQLDriver(ctx, dataSourceName)
	case "postgres", "postgresql":
		return newPostgresDriver(ctx, dataSourceName)
	case "":
		return nil, dsn.ErrMissingScheme
	default:
		scheme := dsn.Scheme(dataSourceName)

		return nil, fmt.Errorf("unsupported DSN scheme %q (supported: mysql, postgres)", scheme)
	}
}
