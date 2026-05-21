package insert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/introspect"
)

// StreamOptions configures the CDC-style continuous-insert loop.
type StreamOptions struct {
	// Rate is the target total rows per second across all tables in order.
	// Must be > 0.
	Rate int
}

// RunStream seeds the schema once (using opts) then keeps appending rows at
// roughly StreamOptions.Rate per second until ctx is cancelled. Each second
// the loop inserts max(Rate/len(order), 1) rows into every table in order
// (integer division; when Rate < len(order) the per-tick total exceeds the
// configured budget — raise --rate above the table count for tighter control).
// Returns ctx.Err() once cancelled; insert errors are returned immediately.
//
// The driver and FK pool are kept across ticks, and per-tick inserts skip
// the full-table Driver.ColumnValues refresh — the pool grows from the rows
// the loop generates itself. DB-managed columns (IDENTITY / serial PKs) do
// not propagate beyond the initial seed in this mode.
func RunStream(
	ctx context.Context,
	dataSourceName string,
	schema introspect.Schema,
	order []string,
	opts Options,
	stream StreamOptions,
	out io.Writer,
) error {
	if stream.Rate <= 0 {
		return errors.New("--rate must be > 0")
	}
	if len(order) == 0 {
		return errors.New("no tables to stream")
	}

	drv, err := openDriver(ctx, dataSourceName, opts, schema, out)
	if err != nil {
		return err
	}
	defer func() { _ = drv.Close(ctx) }()

	byName := make(map[string]introspect.Table, len(schema.Tables))
	for _, t := range schema.Tables {
		byName[t.Name] = t
	}

	pool := NewPool(0)
	poolCols := fkPoolColumns(schema, opts.Polymorphic)

	if opts.Truncate {
		if err := drv.Truncate(ctx, order); err != nil {
			return fmt.Errorf("stream truncate: %w", err)
		}
	}

	var seed uint64
	if opts.Seed != nil {
		seed = *opts.Seed
	} else {
		seed = uint64(time.Now().UnixNano())
	}
	faker := gofakeit.New(seed)

	seedOpts := opts
	seedOpts.Truncate = false

	for _, name := range order {
		t, ok := byName[name]
		if !ok {
			return fmt.Errorf("table %q not in schema", name)
		}
		_, err := insertTable(
			ctx, drv, t, opts.Polymorphic[name], seedOpts,
			faker, pool, poolCols[name], out,
		)
		if err != nil {
			return fmt.Errorf("stream seed %s: %w", name, err)
		}
	}

	perTable := max(stream.Rate/len(order), 1)

	loopOpts := opts
	loopOpts.Truncate = false
	loopOpts.RowsByTable = nil
	loopOpts.Rows = perTable
	loopOpts.Verbose = false
	// Skip the per-tick full-table SELECT; the pool grows from the rows we
	// generate. DB-managed columns are kept at their initial-seed values.
	loopOpts.SkipPoolRefresh = true

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // ctx error is intentionally returned unwrapped
		case <-tick.C:
		}

		// Reseed each tick so UNIQUE generators don't replay the same
		// sequence and collide against rows already inserted.
		faker = gofakeit.New(uint64(time.Now().UnixNano()))

		for _, name := range order {
			t := byName[name]
			_, err := insertTable(
				ctx, drv, t, loopOpts.Polymorphic[name], loopOpts,
				faker, pool, poolCols[name], out,
			)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return ctx.Err() //nolint:wrapcheck // surface cancellation rather than the wrapped insert error
				}

				return fmt.Errorf("stream tick %s: %w", name, err)
			}
		}
	}
}
