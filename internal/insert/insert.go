package insert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/generator"
	"github.com/mickamy/seeder/internal/infer"
	"github.com/mickamy/seeder/internal/introspect"
)

var errNoWritableColumns = errors.New(
	"no writable columns (all columns are identity or DB-managed sequence)",
)

type Options struct {
	// Rows is the default row count; tables listed in RowsByTable override it.
	Rows        int
	RowsByTable map[string]int
	// BatchSize bounds how many rows are generated in memory before a single
	// BulkInsert flush. Zero or negative falls back to 1000.
	BatchSize int
	Truncate  bool
	DryRun    bool
	Verbose   bool
	// Seed is nil for a time-based RNG seed.
	Seed            *uint64
	Locale          infer.Locale
	ColumnOverrides map[string]map[string]ColumnOverride
}

const defaultBatchSize = 1000

type ColumnOverride struct {
	Generator string
	Value     any
}

type Stats struct {
	Table string
	Rows  int64
	Took  time.Duration
}

// fkSpec is shared across every column that belongs to the same foreign key.
// For composite FKs, every member column's colSpec.fk points at the same
// instance, so a row's picker resolves the parent tuple once and spreads
// referenced values across the local columns at consistent indices.
type fkSpec struct {
	referencedTable   string
	referencedColumns []string
	localCols         []string
	// allNullable is true when every local column in the FK is nullable;
	// false aborts insert on an empty parent pool, true degrades to NULLs.
	allNullable bool
}

type colSpec struct {
	name     string
	nullable bool
	// gen is non-nil for seeder-generated columns. FK columns leave it nil
	// and resolve through fk.
	gen   generator.Func
	fk    *fkSpec
	fkIdx int // position within fk.localCols / fk.referencedColumns
}

func Run(
	ctx context.Context,
	dataSourceName string,
	schema introspect.Schema,
	order []string,
	opts Options,
	out io.Writer,
) ([]Stats, error) {
	drv, err := openDriver(ctx, dataSourceName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = drv.Close(ctx) }()

	byName := make(map[string]introspect.Table, len(schema.Tables))
	for _, t := range schema.Tables {
		byName[t.Name] = t
	}

	var seed uint64
	if opts.Seed != nil {
		seed = *opts.Seed
	} else {
		seed = uint64(time.Now().UnixNano())
	}
	faker := gofakeit.New(seed)

	pool := NewPool(0)
	poolCols := fkPoolColumns(schema)

	if opts.Truncate && !opts.DryRun {
		if err := drv.Truncate(ctx, order); err != nil {
			return nil, fmt.Errorf("truncate: %w", err)
		}
	}

	stats := make([]Stats, 0, len(order))
	for _, name := range order {
		t, ok := byName[name]
		if !ok {
			return stats, fmt.Errorf("table %q not in schema", name)
		}
		s, err := insertTable(ctx, drv, t, opts, faker, pool, poolCols[name], out)
		if err != nil {
			return stats, fmt.Errorf("insert %s: %w", name, err)
		}
		stats = append(stats, s)
	}

	return stats, nil
}

func fkPoolColumns(schema introspect.Schema) map[string][]string {
	out := make(map[string][]string, len(schema.Tables))
	add := func(table, col string) {
		if !slices.Contains(out[table], col) {
			out[table] = append(out[table], col)
		}
	}
	for _, t := range schema.Tables {
		for _, pk := range t.PrimaryKey {
			add(t.Name, pk)
		}
	}
	for _, t := range schema.Tables {
		for _, fk := range t.ForeignKeys {
			for _, col := range fk.ReferencedColumns {
				add(fk.ReferencedTable, col)
			}
		}
	}

	return out
}

func planColumns(
	t introspect.Table, faker *gofakeit.Faker,
	locale infer.Locale, overrides map[string]ColumnOverride,
) ([]colSpec, error) {
	nullable := make(map[string]bool, len(t.Columns))
	for _, c := range t.Columns {
		nullable[c.Name] = c.Nullable
	}

	fkByCol := make(map[string]*fkSpec, len(t.ForeignKeys))
	fkIdxByCol := make(map[string]int, len(t.ForeignKeys))
	for _, fk := range t.ForeignKeys {
		spec := &fkSpec{
			referencedTable:   fk.ReferencedTable,
			referencedColumns: slices.Clone(fk.ReferencedColumns),
			localCols:         slices.Clone(fk.Columns),
			allNullable:       true,
		}
		for _, lc := range fk.Columns {
			if !nullable[lc] {
				spec.allNullable = false

				break
			}
		}
		for i, c := range fk.Columns {
			fkByCol[c] = spec
			fkIdxByCol[c] = i
		}
	}

	cols := make([]colSpec, 0, len(t.Columns))
	for _, c := range t.Columns {
		if c.IsIdentity {
			continue
		}
		// FK columns always go through the FK pool, even when they carry a
		// default or a yaml override (the override is intentionally ignored).
		if spec, ok := fkByCol[c.Name]; ok {
			cols = append(cols, colSpec{
				name: c.Name, nullable: c.Nullable,
				fk: spec, fkIdx: fkIdxByCol[c.Name],
			})

			continue
		}
		ov := overrides[c.Name]
		hasOverride := ov.Generator != "" || ov.Value != nil
		// A yaml override wins over the serial-default skip; the user asked
		// for a specific value/generator, so honor it.
		if !hasOverride && isSerialDefault(c.Default) {
			continue
		}

		spec := colSpec{name: c.Name, nullable: c.Nullable}
		if hasOverride {
			gen, err := overrideGenerator(faker, ov)
			if err != nil {
				return nil, fmt.Errorf("column %s: %w", c.Name, err)
			}
			spec.gen = gen
		} else {
			spec.gen = infer.Pick(faker, c, locale)
		}
		cols = append(cols, spec)
	}

	return cols, nil
}

func overrideGenerator(faker *gofakeit.Faker, ov ColumnOverride) (generator.Func, error) {
	switch {
	case ov.Generator != "":
		return generator.ByName(faker, ov.Generator) //nolint:wrapcheck // generator already returns a descriptive error
	case ov.Value != nil:
		v := ov.Value
		return func() any { return v }, nil
	}
	return nil, nil //nolint:nilnil // no override; caller falls back to infer.Pick
}

// isSerialDefault leaves Postgres `nextval(...)` defaults to the DB so the
// sequence stays authoritative; other defaults are intentionally overridden.
func isSerialDefault(def *string) bool {
	return def != nil && strings.HasPrefix(*def, "nextval(")
}

func insertTable(
	ctx context.Context,
	drv Driver,
	t introspect.Table,
	opts Options,
	faker *gofakeit.Faker,
	pool Pool,
	poolCols []string,
	out io.Writer,
) (Stats, error) {
	rows := opts.Rows
	if r, ok := opts.RowsByTable[t.Name]; ok {
		rows = r
	}

	if opts.Verbose {
		explainTable(t, opts.ColumnOverrides[t.Name], out)
	}

	cols, err := planColumns(t, faker, opts.Locale, opts.ColumnOverrides[t.Name])
	if err != nil {
		return Stats{Table: t.Name}, err
	}
	if len(cols) == 0 {
		if rows > 0 && !opts.DryRun {
			return Stats{Table: t.Name}, errNoWritableColumns
		}
		fmt.Fprintf(out, "  %s\tskipped (no writable columns)\n", t.Name)

		return Stats{Table: t.Name}, nil
	}

	if opts.DryRun {
		fmt.Fprintf(out, "  %s\t%d rows (dry-run, columns: %s)\n", t.Name, rows, joinColNames(cols))

		return Stats{Table: t.Name, Rows: int64(rows)}, nil
	}

	// selfFKRefs lists columns referenced by a self-FK in this table; only
	// those values are stashed per row so later rows in the same batch can
	// pick them.
	selfFKRefs := make(map[string]bool)
	for _, c := range cols {
		if c.fk != nil && c.fk.referencedTable == t.Name {
			for _, refCol := range c.fk.referencedColumns {
				selfFKRefs[refCol] = true
			}
		}
	}

	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.name
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	// inBatch is kept across batches so self-FK forward-reference behaves the
	// same regardless of --batch-size (the seeded output stays deterministic).
	inBatch := newBatchBuffer(selfFKRefs, defaultPoolCapacity)

	var totalInserted int64
	var totalTook time.Duration
	remaining := rows
	for remaining > 0 {
		b := min(remaining, batchSize)
		data, err := generateBatch(b, cols, faker, pool, inBatch, t.Name)
		if err != nil {
			return Stats{Table: t.Name, Rows: totalInserted, Took: totalTook}, err
		}

		start := time.Now()
		n, err := drv.BulkInsert(ctx, t.Name, colNames, data)
		if err != nil {
			return Stats{Table: t.Name, Rows: totalInserted, Took: totalTook}, fmt.Errorf("bulk insert: %w", err)
		}
		totalTook += time.Since(start)
		totalInserted += n
		remaining -= b
	}

	if len(poolCols) > 0 {
		vals, err := drv.ColumnValues(ctx, t.Name, poolCols)
		if err != nil {
			return Stats{Table: t.Name, Rows: totalInserted, Took: totalTook}, fmt.Errorf("column values: %w", err)
		}
		pool.Replace(t.Name, vals)
	}

	fmt.Fprintf(out, "  %s\t%d rows (%s)\n", t.Name, totalInserted, totalTook.Truncate(time.Microsecond))

	return Stats{Table: t.Name, Rows: totalInserted, Took: totalTook}, nil
}

// batchBuffer accumulates the columns referenced by self-FKs in this table,
// row by row, so later rows in the same batch can forward-reference them.
// Only refs-listed columns are remembered. The buffer is tail-trimmed to
// cap, matching Pool's capacity policy.
type batchBuffer struct {
	refs map[string]bool
	rows []map[string]any
	cap  int
}

func newBatchBuffer(refs map[string]bool, cap int) *batchBuffer {
	return &batchBuffer{refs: refs, cap: cap}
}

func (b *batchBuffer) push(row map[string]any) {
	if len(row) == 0 {
		return
	}
	b.rows = append(b.rows, row)
	if len(b.rows) > b.cap {
		b.rows = b.rows[len(b.rows)-b.cap:]
	}
}

func (b *batchBuffer) len() int                  { return len(b.rows) }
func (b *batchBuffer) at(i int) map[string]any   { return b.rows[i] }
func (b *batchBuffer) refSet() map[string]bool   { return b.refs }

func generateBatch(
	n int,
	cols []colSpec,
	faker *gofakeit.Faker,
	pool Pool,
	inBatch *batchBuffer,
	tableName string,
) ([][]any, error) {
	data := make([][]any, 0, n)
	for range n {
		row := make([]any, len(cols))

		for j, c := range cols {
			if c.gen != nil {
				row[j] = c.gen()
			}
		}

		// Each FK group resolves once per row; the chosen parent tuple is
		// then spread across the FK's local columns.
		picked := make(map[*fkSpec]map[string]any)
		absent := make(map[*fkSpec]bool)
		for j, c := range cols {
			if c.gen != nil {
				continue
			}
			if _, done := picked[c.fk]; !done && !absent[c.fk] {
				var pickedRow map[string]any
				var err error
				if c.fk.referencedTable == tableName {
					pickedRow, err = pickSelfFKRow(faker, pool, inBatch, row, cols, c.fk, tableName)
				} else {
					pickedRow, err = pickFKRow(faker, pool, c.fk, tableName)
				}
				if err != nil {
					return nil, err
				}
				if pickedRow == nil {
					absent[c.fk] = true
				} else {
					picked[c.fk] = pickedRow
				}
			}
			if absent[c.fk] {
				row[j] = nil

				continue
			}
			row[j] = picked[c.fk][c.fk.referencedColumns[c.fkIdx]]
		}

		if refs := inBatch.refSet(); len(refs) > 0 {
			stash := make(map[string]any, len(refs))
			for j, c := range cols {
				if refs[c.name] {
					stash[c.name] = row[j]
				}
			}
			inBatch.push(stash)
		}

		data = append(data, row)
	}

	return data, nil
}

func pickFKRow(faker *gofakeit.Faker, pool Pool, fk *fkSpec, tableName string) (map[string]any, error) {
	row := pool.PickRow(faker, fk.referencedTable)
	if row != nil {
		return row, nil
	}
	if !fk.allNullable {
		return nil, fmt.Errorf(
			"FK target %s has no rows but %s requires a value for (%s)",
			fk.referencedTable, tableName, strings.Join(fk.localCols, ", "),
		)
	}

	return nil, nil //nolint:nilnil // intentional NULL group for a fully-nullable FK with empty parent pool
}

// pickSelfFKRow combines the parent pool and the in-batch buffer at a single
// uniform index, so forward-references behave the same across --batch-size.
// The last-chance self-loop applies only when every referenced column is
// seeder-generated in the current row.
func pickSelfFKRow(
	faker *gofakeit.Faker,
	pool Pool,
	inBatch *batchBuffer,
	row []any,
	cols []colSpec,
	fk *fkSpec,
	tableName string,
) (map[string]any, error) {
	poolRows := pool.Rows(fk.referencedTable)
	total := len(poolRows) + inBatch.len()
	if total > 0 {
		idx := faker.IntRange(0, total-1)
		if idx < len(poolRows) {
			return poolRows[idx], nil
		}

		return inBatch.at(idx - len(poolRows)), nil
	}

	if fk.allNullable {
		return nil, nil //nolint:nilnil // intentional NULL group for fully-nullable self-FK on the first row
	}

	selfRow := make(map[string]any, len(fk.referencedColumns))
	for _, refCol := range fk.referencedColumns {
		v, ok := lookupOwnRef(row, cols, refCol)
		if !ok {
			return nil, fmt.Errorf(
				"self-FK on %s (%s) is NOT NULL but no seeded values are available and %s.%s is not seeder-generated",
				tableName, strings.Join(fk.localCols, ", "),
				fk.referencedTable, refCol,
			)
		}
		selfRow[refCol] = v
	}

	return selfRow, nil
}

func lookupOwnRef(row []any, cols []colSpec, refCol string) (any, bool) {
	for j, c := range cols {
		if c.name == refCol && c.gen != nil {
			return row[j], true
		}
	}

	return nil, false
}

func joinColNames(cols []colSpec) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.name
	}

	return strings.Join(names, ", ")
}

func explainTable(t introspect.Table, overrides map[string]ColumnOverride, out io.Writer) {
	fmt.Fprintf(out, "  %s\n", t.Name)
	for _, c := range t.Columns {
		fmt.Fprintf(out, "    %s\t%s\n", c.Name, explainColumn(t, c, overrides[c.Name]))
	}
}

func explainColumn(t introspect.Table, c introspect.Column, ov ColumnOverride) string {
	if c.IsIdentity {
		return "skip: identity"
	}
	for _, fk := range t.ForeignKeys {
		for i, lc := range fk.Columns {
			if lc == c.Name {
				return fmt.Sprintf("fk: %s.%s", fk.ReferencedTable, fk.ReferencedColumns[i])
			}
		}
	}
	hasOverride := ov.Generator != "" || ov.Value != nil
	if !hasOverride && isSerialDefault(c.Default) {
		return "skip: serial default"
	}
	switch {
	case ov.Generator != "":
		return "override: generator=" + ov.Generator
	case ov.Value != nil:
		return fmt.Sprintf("override: value=%v", ov.Value)
	}

	return infer.Explain(c)
}
