package insert

const defaultPoolCapacity = 100_000

// Pool stores parent column values that FK columns pick from. Replace caps
// each per-column slice at Capacity by keeping its tail, so memory stays
// bounded under large inserts. The trimming is "keep the last N", not LRU:
// callers (typically Driver.ColumnValues) decide the ordering of the input
// slice.
//
// Construct via NewPool; the zero value is not usable.
type Pool struct {
	Capacity int
	data     map[string]map[string][]any
}

func NewPool(capacity int) Pool {
	if capacity <= 0 {
		capacity = defaultPoolCapacity
	}

	return Pool{Capacity: capacity, data: make(map[string]map[string][]any)}
}

func (p Pool) Values(table, col string) []any {
	return p.data[table][col]
}

// Replace overwrites the per-column slices for table with vals, keeping at
// most the tail Capacity entries of each. Panics on a zero-value Pool.
func (p Pool) Replace(table string, vals map[string][]any) {
	if p.data == nil {
		panic("insert: Pool must be constructed via NewPool")
	}
	if p.data[table] == nil {
		p.data[table] = make(map[string][]any, len(vals))
	}
	for col, vs := range vals {
		if p.Capacity > 0 && len(vs) > p.Capacity {
			vs = vs[len(vs)-p.Capacity:]
		}
		cp := make([]any, len(vs))
		copy(cp, vs)
		p.data[table][col] = cp
	}
}
