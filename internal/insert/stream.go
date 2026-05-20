package insert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

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
// the loop inserts ceil(Rate/len(order)) rows into every table, in order.
// Returns ctx.Err() (or nil for nil ctx) once cancelled; insert errors are
// returned immediately.
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

	if _, err := Run(ctx, dataSourceName, schema, order, opts, out); err != nil {
		return fmt.Errorf("stream seed: %w", err)
	}

	perTable := max(stream.Rate/len(order), 1)

	loopOpts := opts
	loopOpts.Truncate = false
	loopOpts.RowsByTable = nil
	loopOpts.Rows = perTable
	loopOpts.Verbose = false
	// Reseed each tick so UNIQUE generators don't replay the same sequence
	// (which would collide against rows the seed run already wrote).
	loopOpts.Seed = nil

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // ctx error is intentionally returned unwrapped
		case <-tick.C:
		}

		if _, err := Run(ctx, dataSourceName, schema, order, loopOpts, out); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ctx.Err() //nolint:wrapcheck // surface the cancellation rather than the wrapped insert error
			}

			return fmt.Errorf("stream tick: %w", err)
		}
	}
}
