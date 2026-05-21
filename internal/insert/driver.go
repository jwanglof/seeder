package insert

import (
	"context"
	"fmt"
	"io"

	"github.com/mickamy/seeder/internal/dsn"
	"github.com/mickamy/seeder/internal/introspect"
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
// schema feeds the SQL output driver so it can emit dialect-specific clauses
// (e.g., OVERRIDING SYSTEM VALUE on Postgres tables with IDENTITY columns).
func openDriver(
	ctx context.Context,
	dataSourceName string,
	opts Options,
	schema introspect.Schema,
	defaultOut io.Writer,
) (Driver, error) {
	scheme := dsn.Scheme(dataSourceName)
	if opts.OutputMode != "" {
		w := opts.OutputWriter
		if w == nil {
			w = defaultOut
		}
		switch opts.OutputMode {
		case "sql":
			if scheme != "mysql" && scheme != "postgres" && scheme != "postgresql" {
				return nil, fmt.Errorf(
					"--output=sql needs a mysql:// / postgres:// / postgresql:// DSN to choose the SQL dialect (got %q)",
					scheme,
				)
			}
			return newSQLOutputDriver(w, scheme, schema), nil
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
