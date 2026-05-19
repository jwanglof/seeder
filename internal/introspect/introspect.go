package introspect

import (
	"context"
	"fmt"

	"github.com/mickamy/seeder/internal/dsn"
)

type Driver interface {
	Close(ctx context.Context) error
	Introspect(ctx context.Context) (*Schema, error)
}

func Do(ctx context.Context, dataSourceName string) (*Schema, error) {
	drv, err := openDriver(ctx, dataSourceName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = drv.Close(ctx) }()

	schema, err := drv.Introspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect: %w", err)
	}

	return schema, nil
}

func openDriver(ctx context.Context, dataSourceName string) (Driver, error) {
	switch dsn.Scheme(dataSourceName) {
	case "postgres", "postgresql":
		return newPostgresDriver(ctx, dataSourceName)
	case "":
		return nil, dsn.ErrMissingScheme
	default:
		return nil, fmt.Errorf("unsupported DSN scheme %q; supported: postgres, postgresql", dsn.Scheme(dataSourceName))
	}
}
