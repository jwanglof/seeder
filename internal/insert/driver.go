package insert

import (
	"context"
	"fmt"
	"io"

	"github.com/mickamy/seeder/internal/dsn"
)

type Driver interface {
	Close(ctx context.Context) error
	Truncate(ctx context.Context, tables []string) error
	BulkInsert(ctx context.Context, table string, columns []string, rows [][]any) (int64, error)
	ColumnValues(ctx context.Context, table string, columns []string) (map[string][]any, error)
}

// openDriver dispatches between DB drivers and the alternate output drivers.
// When opts.OutputMode is set the dataSourceName is only used to pick the SQL
// dialect (no connection is opened); otherwise a real DB driver is returned.
func openDriver(ctx context.Context, dataSourceName string, opts Options, defaultOut io.Writer) (Driver, error) {
	scheme := dsn.Scheme(dataSourceName)
	if opts.OutputMode != "" {
		w := opts.OutputWriter
		if w == nil {
			w = defaultOut
		}
		switch opts.OutputMode {
		case "sql":
			if scheme != "mysql" && scheme != "postgres" && scheme != "postgresql" {
				return nil, fmt.Errorf("--output=sql needs mysql:// or postgres:// to choose the SQL dialect (got %q)", scheme)
			}
			return newSQLOutputDriver(w, scheme), nil
		case "ndjson":
			return newNDJSONOutputDriver(w), nil
		default:
			return nil, fmt.Errorf("--output: unknown mode %q (supported: ndjson, sql)", opts.OutputMode)
		}
	}

	switch scheme {
	case "mysql":
		return newMySQLDriver(ctx, dataSourceName)
	case "postgres", "postgresql":
		return newPostgresDriver(ctx, dataSourceName)
	case "":
		return nil, dsn.ErrMissingScheme
	default:
		return nil, fmt.Errorf("unsupported DSN scheme %q (supported: mysql, postgres, postgresql)", scheme)
	}
}
