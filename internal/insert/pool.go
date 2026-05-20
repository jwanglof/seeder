package insert

const defaultPoolCapacity = 100_000

// Pool stores parent column values that FK columns pick from. It caps the
// per-(table, column) ring at Capacity to keep memory bounded under large
// inserts; older values are dropped first.
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

// Replace overwrites the per-column rings for table with vals, capping each at
// Capacity (keeping the tail).
func (p Pool) Replace(table string, vals map[string][]any) {
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
