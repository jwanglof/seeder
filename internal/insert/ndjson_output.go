package insert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// ndjsonOutputDriver writes one JSON object per row to an io.Writer. Each
// line carries a "_table" field so downstream consumers can route rows by
// destination. Rows are retained in-memory to back ColumnValues (FK and
// polymorphic pickers read from this).
type ndjsonOutputDriver struct {
	w       io.Writer
	written map[string][]map[string]any
}

func newNDJSONOutputDriver(w io.Writer) *ndjsonOutputDriver {
	return &ndjsonOutputDriver{
		w:       w,
		written: make(map[string][]map[string]any),
	}
}

func (d *ndjsonOutputDriver) Close(_ context.Context) error { return nil }

// Truncate is a no-op: NDJSON is a row stream, so there's no schema-level
// reset to express.
func (d *ndjsonOutputDriver) Truncate(_ context.Context, _ []string) error {
	return nil
}

func (d *ndjsonOutputDriver) BulkInsert(
	_ context.Context, table string, columns []string, rows [][]any,
) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	enc := json.NewEncoder(d.w)
	for _, row := range rows {
		obj := make(map[string]any, len(columns)+1)
		obj["_table"] = table
		for i, c := range columns {
			obj[c] = row[i]
		}
		if err := enc.Encode(obj); err != nil {
			return 0, fmt.Errorf("ndjson encode: %w", err)
		}
	}

	d.written[table] = appendBoundedRows(d.written[table], columns, rows, defaultPoolCapacity)

	return int64(len(rows)), nil
}

func (d *ndjsonOutputDriver) ColumnValues(_ context.Context, table string, columns []string) (map[string][]any, error) {
	out := make(map[string][]any, len(columns))
	for _, row := range d.written[table] {
		for _, c := range columns {
			out[c] = append(out[c], row[c])
		}
	}

	return out, nil
}
