package insert

import (
	"fmt"
	"strings"
)

// maxCompositeUniqueAttempts caps how many times generateBatch will regenerate
// a single row trying to dodge a composite UNIQUE collision. Past that, the
// column cardinality is almost certainly too low to hold the requested row
// count and the caller has to reduce --rows or exclude the table.
const maxCompositeUniqueAttempts = 16

// compositeUniqueSpec tracks one multi-column UNIQUE constraint while
// generating rows for a table: colIdx points at the columns participating in
// the constraint inside the current cols slice, and seen remembers every
// tuple we have emitted so far so we can reject duplicates before the DB
// would (Error 1062 on MySQL, 23505 on Postgres).
type compositeUniqueSpec struct {
	constraint []string
	colIdx     []int
	seen       map[string]bool
}

// newCompositeUniques resolves each constraint's column names to indexes
// inside cols. Constraints that reference a column seeder does not write
// (identity, dropped by overrides, ...) are skipped — without writing the
// column there is nothing to deduplicate against.
func newCompositeUniques(cols []colSpec, constraints [][]string) []*compositeUniqueSpec {
	if len(constraints) == 0 {
		return nil
	}
	byName := make(map[string]int, len(cols))
	for i, c := range cols {
		byName[c.name] = i
	}
	out := make([]*compositeUniqueSpec, 0, len(constraints))
	for _, cnames := range constraints {
		idx := make([]int, 0, len(cnames))
		ok := true
		for _, cn := range cnames {
			found, present := byName[cn]
			if !present {
				ok = false

				break
			}
			idx = append(idx, found)
		}
		if !ok {
			continue
		}
		out = append(out, &compositeUniqueSpec{
			constraint: cnames,
			colIdx:     idx,
			seen:       make(map[string]bool),
		})
	}

	return out
}

// compositeKey serializes the values at colIdx into a string usable as a
// map key. The 0x1f (unit separator) byte is chosen because real seed values
// effectively never contain it, so distinct tuples cannot collide via
// concatenation.
func compositeKey(row []any, colIdx []int) string {
	var b strings.Builder
	for i, idx := range colIdx {
		if i > 0 {
			b.WriteByte(0x1f)
		}
		fmt.Fprintf(&b, "%v", row[idx])
	}

	return b.String()
}
