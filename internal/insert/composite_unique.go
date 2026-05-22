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
// map key. Each value is length-prefixed so distinct tuples cannot collide
// even if a value happens to contain the separator character.
func compositeKey(row []any, colIdx []int) string {
	var b strings.Builder
	for _, idx := range colIdx {
		v := fmt.Sprintf("%v", row[idx])
		fmt.Fprintf(&b, "%d:%s", len(v), v)
	}

	return b.String()
}

// compositeKeyForRow returns the dedup key for a row's constraint columns,
// or skip=true when any participating column is nil. SQL UNIQUE treats NULL
// participants as never colliding (multiple NULLs are allowed under both
// MySQL and Postgres), so callers must skip dedup for those tuples.
func compositeKeyForRow(row []any, colIdx []int) (string, bool) {
	for _, idx := range colIdx {
		if row[idx] == nil {
			return "", true
		}
	}

	return compositeKey(row, colIdx), false
}
