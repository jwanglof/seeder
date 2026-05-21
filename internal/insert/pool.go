package insert

import "github.com/brianvoe/gofakeit/v7"

const defaultPoolCapacity = 100_000

// Pool stores parent table rows that FK columns pick from. Rows are kept as
// whole tuples (one map per row) so composite-FK callers can grab every
// referenced column at the same index. Replace tail-trims to Capacity,
// keeping the most recent rows; callers (typically Driver.ColumnValues)
// decide ordering of the source slices.
//
// Construct via NewPool; the zero value is not usable.
type Pool struct {
	Capacity int
	data     map[string][]map[string]any
}

func NewPool(capacity int) Pool {
	if capacity <= 0 {
		capacity = defaultPoolCapacity
	}

	return Pool{Capacity: capacity, data: make(map[string][]map[string]any)}
}

// Values returns col across every row in table, in row order. Rows missing
// the column contribute a nil entry. Returns nil when table has no rows.
func (p Pool) Values(table, col string) []any {
	rows := p.data[table]
	if len(rows) == 0 {
		return nil
	}
	out := make([]any, len(rows))
	for i, row := range rows {
		out[i] = row[col]
	}

	return out
}

// PickRow returns a uniformly random row from table, or nil if table has
// none. The returned map is the live entry; callers must not mutate it.
func (p Pool) PickRow(faker *gofakeit.Faker, table string) map[string]any {
	rows := p.data[table]
	if len(rows) == 0 {
		return nil
	}

	return rows[faker.IntRange(0, len(rows)-1)]
}

// Rows exposes the live row slice for table (nil when empty). The slice and
// the row maps are read-only; mutation breaks future picks. Used by self-FK
// pickers that need to combine pool rows with an in-batch buffer at a single
// uniform index.
func (p Pool) Rows(table string) []map[string]any {
	return p.data[table]
}

// Append adds rows to table's pool and tail-trims to Capacity. Stream mode
// uses this so it doesn't have to re-fetch every row from the DB each tick.
// Panics on a zero-value Pool.
func (p Pool) Append(table string, rows []map[string]any) {
	if p.data == nil {
		panic("insert: Pool must be constructed via NewPool")
	}
	if len(rows) == 0 {
		return
	}
	p.data[table] = append(p.data[table], rows...)
	if p.Capacity > 0 && len(p.data[table]) > p.Capacity {
		p.data[table] = p.data[table][len(p.data[table])-p.Capacity:]
	}
}

// Replace overwrites the rows for table by zipping the per-column slices in
// vals into row tuples. Each vals[col][i] must come from the same source row
// (holds when Driver.ColumnValues fills the map from a single SELECT). The
// result is tail-trimmed to Capacity. Panics on a zero-value Pool.
func (p Pool) Replace(table string, vals map[string][]any) {
	if p.data == nil {
		panic("insert: Pool must be constructed via NewPool")
	}
	n := 0
	for _, vs := range vals {
		if len(vs) > n {
			n = len(vs)
		}
	}
	start := 0
	if p.Capacity > 0 && n > p.Capacity {
		start = n - p.Capacity
	}
	rows := make([]map[string]any, 0, n-start)
	for i := start; i < n; i++ {
		row := make(map[string]any, len(vals))
		for col, vs := range vals {
			if i < len(vs) {
				row[col] = vs[i]
			}
		}
		rows = append(rows, row)
	}
	p.data[table] = rows
}
