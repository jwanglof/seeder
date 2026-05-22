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
	// Polymorphic maps a table to its declared polymorphic associations.
	// Targets are pre-resolved by the caller (CLI), so IDCol is always set.
	Polymorphic map[string][]PolymorphicSpec
	// OutputMode redirects writes instead of touching the database.
	// "" → DB insert (default), "sql" → INSERT statements, "ndjson" → NDJSON.
	OutputMode string
	// OutputWriter is the destination for OutputMode != "". When nil, the
	// out writer passed to Run is used.
	OutputWriter io.Writer
	// SkipPoolRefresh keeps insertTable from issuing Driver.ColumnValues
	// after every table flush. CDC streaming sets it so each tick stays
	// O(batch) instead of O(table). The pool is fed from the rows just
	// inserted (seeder-generated columns only); DB-managed columns such as
	// IDENTITY / serial PKs do NOT propagate into the pool in this mode, so
	// later FKs may stick to the values captured during the initial seed.
	SkipPoolRefresh bool
}

const defaultBatchSize = 1000

type ColumnOverride struct {
	Generator string
	Value     any
}

// PolymorphicSpec describes a Rails-style polymorphic association on the
// owning table: TypeColumn holds Target.Type, IDColumn holds the picked
// row's IDCol value. Resolved by the CLI from PolymorphicConfig in yaml.
type PolymorphicSpec struct {
	TypeColumn string
	IDColumn   string
	Targets    []PolymorphicTarget
}

type PolymorphicTarget struct {
	Table string
	Type  string
	IDCol string
}

type Stats struct {
	Table string
	Rows  int64
	Took  time.Duration
}

// fkSpec is shared across every column belonging to the same foreign key.
// For composite FKs every member column's colSpec.fk points at the same
// instance, so a row's picker resolves the parent tuple once and spreads
// referenced values across the local columns at consistent indices.
type fkSpec struct {
	referencedTable   string
	referencedColumns []string
	localCols         []string
	allNullable       bool
}

type colSpec struct {
	name     string
	nullable bool
	// gen is non-nil for seeder-generated columns. FK and polymorphic
	// columns leave it nil and resolve through fk / poly.
	gen      generator.Func
	fk       *fkSpec
	fkIdx    int // position within fk.localCols / fk.referencedColumns
	poly     *polySpec
	polyKind polyColKind
}

type polySpec struct {
	typeCol     string
	idCol       string
	targets     []polyTarget
	allNullable bool
}

type polyTarget struct {
	table string
	typ   string
	idCol string
}

type polyColKind int

const (
	polyColNone polyColKind = iota
	polyColType
	polyColID
)

func Run(
	ctx context.Context,
	dataSourceName string,
	schema introspect.Schema,
	order []string,
	opts Options,
	out io.Writer,
) ([]Stats, error) {
	drv, err := openDriver(ctx, dataSourceName, opts, schema, out)
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
	poolCols := fkPoolColumns(schema, opts.Polymorphic)

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
		s, err := insertTable(ctx, drv, t, opts.Polymorphic[name], opts, faker, pool, poolCols[name], out)
		if err != nil {
			return stats, fmt.Errorf("insert %s: %w", name, err)
		}
		stats = append(stats, s)
	}

	return stats, nil
}

func fkPoolColumns(schema introspect.Schema, polys map[string][]PolymorphicSpec) map[string][]string {
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
	for _, ps := range polys {
		for _, p := range ps {
			for _, target := range p.Targets {
				add(target.Table, target.IDCol)
			}
		}
	}

	return out
}

// planColumns inspects t and returns the columns seeder will fill.
//
// keepDBManaged controls whether IDENTITY columns and Postgres `nextval(...)`
// defaults are seeder-generated. In normal DB mode the DB owns those values
// so we skip them; in output mode (--output sql / ndjson) there is no DB to
// assign them, so we generate values ourselves to keep downstream FK / poly
// pools populated.
func planColumns(
	t introspect.Table,
	polys []PolymorphicSpec,
	faker *gofakeit.Faker,
	locale infer.Locale,
	overrides map[string]ColumnOverride,
	keepDBManaged bool,
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

	polyByCol := make(map[string]*polySpec, len(polys))
	polyKindByCol := make(map[string]polyColKind, len(polys))
	for _, p := range polys {
		spec := &polySpec{
			typeCol:     p.TypeColumn,
			idCol:       p.IDColumn,
			allNullable: nullable[p.TypeColumn] && nullable[p.IDColumn],
		}
		for _, tg := range p.Targets {
			spec.targets = append(spec.targets, polyTarget{
				table: tg.Table,
				typ:   tg.Type,
				idCol: tg.IDCol,
			})
		}
		polyByCol[p.TypeColumn] = spec
		polyKindByCol[p.TypeColumn] = polyColType
		polyByCol[p.IDColumn] = spec
		polyKindByCol[p.IDColumn] = polyColID
	}

	cols := make([]colSpec, 0, len(t.Columns))
	for _, c := range t.Columns {
		if c.IsGenerated {
			continue
		}
		if c.IsIdentity && !keepDBManaged {
			continue
		}
		if spec, ok := fkByCol[c.Name]; ok {
			cols = append(cols, colSpec{
				name: c.Name, nullable: c.Nullable,
				fk: spec, fkIdx: fkIdxByCol[c.Name],
			})

			continue
		}
		if spec, ok := polyByCol[c.Name]; ok {
			cols = append(cols, colSpec{
				name: c.Name, nullable: c.Nullable,
				poly: spec, polyKind: polyKindByCol[c.Name],
			})

			continue
		}

		ov := overrides[c.Name]
		hasOverride := ov.Generator != "" || ov.Value != nil
		if !hasOverride && isSerialDefault(c.Default) && !keepDBManaged {
			continue
		}

		// In output mode the DB does not assign identity / serial values, so
		// the seeder has to pick them. Single-column primary keys are unique
		// by definition; flag them so the generator picks a collision-aware
		// strategy and the emitted SQL/NDJSON respects the PK constraint.
		col := c
		if keepDBManaged && !col.IsUnique &&
			(col.IsIdentity || isSerialDefault(col.Default)) &&
			isSingleColumnPK(t, col.Name) {
			col.IsUnique = true
		}

		spec := colSpec{name: col.Name, nullable: col.Nullable}
		if hasOverride {
			gen, err := overrideGenerator(faker, ov)
			if err != nil {
				return nil, fmt.Errorf("column %s: %w", col.Name, err)
			}
			spec.gen = gen
		} else {
			spec.gen = infer.Pick(faker, col, locale)
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

func isSingleColumnPK(t introspect.Table, name string) bool {
	return len(t.PrimaryKey) == 1 && t.PrimaryKey[0] == name
}

// poolRowsFromBatch projects a freshly-generated batch onto the pool-column
// set. Used by stream mode to feed the pool without re-fetching from the DB.
// If any pool column is DB-managed (identity / serial) — i.e., not present
// in cols — the function returns nil so the caller leaves the table's pool
// alone. Emitting rows with missing columns would let downstream FK pickers
// grab partial tuples and write NULLs for the missing referenced column.
func poolRowsFromBatch(batch [][]any, cols []colSpec, poolCols []string) []map[string]any {
	if len(batch) == 0 || len(poolCols) == 0 {
		return nil
	}
	want := make(map[string]int, len(poolCols))
	for _, c := range poolCols {
		want[c] = -1
	}
	for i, c := range cols {
		if _, ok := want[c.name]; ok {
			want[c.name] = i
		}
	}
	for _, idx := range want {
		if idx < 0 {
			return nil
		}
	}
	out := make([]map[string]any, 0, len(batch))
	for _, row := range batch {
		m := make(map[string]any, len(poolCols))
		for col, idx := range want {
			m[col] = row[idx]
		}
		out = append(out, m)
	}

	return out
}

func insertTable(
	ctx context.Context,
	drv Driver,
	t introspect.Table,
	polys []PolymorphicSpec,
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
		explainTable(t, polys, opts.ColumnOverrides[t.Name], opts.OutputMode != "", out)
	}

	cols, err := planColumns(t, polys, faker, opts.Locale, opts.ColumnOverrides[t.Name], opts.OutputMode != "")
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

	inBatch := newBatchBuffer(selfFKRefs, defaultPoolCapacity)
	composites := newCompositeUniques(cols, t.CompositeUniques)

	var totalInserted int64
	var totalTook time.Duration
	remaining := rows
	for remaining > 0 {
		b := min(remaining, batchSize)
		data, err := generateBatch(b, cols, faker, pool, inBatch, composites, t.Name)
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

		if opts.SkipPoolRefresh && len(poolCols) > 0 {
			pool.Append(t.Name, poolRowsFromBatch(data, cols, poolCols))
		}
	}

	if !opts.SkipPoolRefresh && len(poolCols) > 0 {
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

func newBatchBuffer(refs map[string]bool, capacity int) *batchBuffer {
	return &batchBuffer{refs: refs, cap: capacity}
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

func (b *batchBuffer) refSet() map[string]bool { return b.refs }

// rowsCovering returns the subset of buffered rows that hold every column
// in cols. Self-FK pickers use this so they never pick an inBatch row
// missing a column they need (which would otherwise emit a NULL into a
// NOT NULL self-FK target).
func (b *batchBuffer) rowsCovering(cols []string) []map[string]any {
	out := make([]map[string]any, 0, len(b.rows))
outer:
	for _, row := range b.rows {
		for _, c := range cols {
			if _, ok := row[c]; !ok {
				continue outer
			}
		}
		out = append(out, row)
	}

	return out
}

func generateBatch(
	n int,
	cols []colSpec,
	faker *gofakeit.Faker,
	pool Pool,
	inBatch *batchBuffer,
	composites []*compositeUniqueSpec,
	tableName string,
) ([][]any, error) {
	data := make([][]any, 0, n)
	keys := make([]string, 0, len(composites))
	skipFlags := make([]bool, 0, len(composites))
	for range n {
		var row []any
		for attempts := 0; ; attempts++ {
			var err error
			row, err = generateOneRow(cols, faker, pool, inBatch, tableName)
			if err != nil {
				return nil, err
			}

			keys = keys[:0]
			skipFlags = skipFlags[:0]
			collision := false
			var collisionCU *compositeUniqueSpec
			for _, cu := range composites {
				key, skip := compositeKeyForRow(row, cu.colIdx)
				keys = append(keys, key)
				skipFlags = append(skipFlags, skip)
				if skip {
					continue
				}
				if cu.seen[key] {
					collision = true
					collisionCU = cu

					break
				}
			}
			if !collision {
				for i, cu := range composites {
					if skipFlags[i] {
						continue
					}
					cu.seen[keys[i]] = true
				}

				break
			}
			if attempts+1 >= maxCompositeUniqueAttempts {
				return nil, fmt.Errorf(
					"composite UNIQUE (%s) collision in %s after %d attempts at tuple (%s); reduce --rows or --exclude %s",
					strings.Join(collisionCU.constraint, ", "),
					tableName,
					maxCompositeUniqueAttempts,
					formatCollisionTuple(row, collisionCU.colIdx),
					tableName,
				)
			}
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

func generateOneRow(
	cols []colSpec,
	faker *gofakeit.Faker,
	pool Pool,
	inBatch *batchBuffer,
	tableName string,
) ([]any, error) {
	row := make([]any, len(cols))

	for j, c := range cols {
		if c.gen != nil {
			row[j] = c.gen()
		}
	}

	pickedFK := make(map[*fkSpec]map[string]any)
	absentFK := make(map[*fkSpec]bool)
	for j, c := range cols {
		if c.gen != nil || c.fk == nil {
			continue
		}
		if _, done := pickedFK[c.fk]; !done && !absentFK[c.fk] {
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
				absentFK[c.fk] = true
			} else {
				pickedFK[c.fk] = pickedRow
			}
		}
		if absentFK[c.fk] {
			row[j] = nil

			continue
		}
		row[j] = pickedFK[c.fk][c.fk.referencedColumns[c.fkIdx]]
	}

	pickedPoly := make(map[*polySpec]polyResolved)
	absentPoly := make(map[*polySpec]bool)
	for j, c := range cols {
		if c.poly == nil {
			continue
		}
		if _, done := pickedPoly[c.poly]; !done && !absentPoly[c.poly] {
			resolved, err := pickPolymorphic(faker, pool, c.poly, tableName)
			if err != nil {
				return nil, err
			}
			if resolved == (polyResolved{}) {
				absentPoly[c.poly] = true
			} else {
				pickedPoly[c.poly] = resolved
			}
		}
		if absentPoly[c.poly] {
			row[j] = nil

			continue
		}
		p := pickedPoly[c.poly]
		if c.polyKind == polyColType {
			row[j] = p.typ
		} else {
			row[j] = p.id
		}
	}

	return row, nil
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
	// Only consider in-batch rows that carry every column this FK references.
	// When a referenced column is DB-managed (or absent for another reason),
	// its value is unknown until after the insert, so the corresponding rows
	// must not feed this FK.
	available := inBatch.rowsCovering(fk.referencedColumns)
	total := len(poolRows) + len(available)
	if total > 0 {
		idx := faker.IntRange(0, total-1)
		if idx < len(poolRows) {
			return poolRows[idx], nil
		}

		return available[idx-len(poolRows)], nil
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

type polyResolved struct {
	typ string
	id  any
}

func pickPolymorphic(
	faker *gofakeit.Faker,
	pool Pool,
	spec *polySpec,
	tableName string,
) (polyResolved, error) {
	if len(spec.targets) == 0 {
		return polyResolved{}, fmt.Errorf(
			"polymorphic spec on %s.(%s, %s) has no targets",
			tableName, spec.typeCol, spec.idCol,
		)
	}
	target := spec.targets[faker.IntRange(0, len(spec.targets)-1)]
	row := pool.PickRow(faker, target.table)
	if row == nil {
		if spec.allNullable {
			return polyResolved{}, nil
		}

		return polyResolved{}, fmt.Errorf(
			"polymorphic target %s has no rows but %s.(%s, %s) requires a value",
			target.table, tableName, spec.typeCol, spec.idCol,
		)
	}

	return polyResolved{typ: target.typ, id: row[target.idCol]}, nil
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

func explainTable(
	t introspect.Table,
	polys []PolymorphicSpec,
	overrides map[string]ColumnOverride,
	keepDBManaged bool,
	out io.Writer,
) {
	fmt.Fprintf(out, "  %s\n", t.Name)
	for _, c := range t.Columns {
		fmt.Fprintf(out, "    %s\t%s\n", c.Name, explainColumn(t, polys, c, overrides[c.Name], keepDBManaged))
	}
}

func explainColumn(
	t introspect.Table,
	polys []PolymorphicSpec,
	c introspect.Column,
	ov ColumnOverride,
	keepDBManaged bool,
) string {
	if c.IsGenerated {
		return "skip: generated"
	}
	if c.IsIdentity {
		if keepDBManaged {
			return "generated: identity (output mode)"
		}
		return "skip: identity"
	}
	for _, fk := range t.ForeignKeys {
		for i, lc := range fk.Columns {
			if lc == c.Name {
				return fmt.Sprintf("fk: %s.%s", fk.ReferencedTable, fk.ReferencedColumns[i])
			}
		}
	}
	for _, p := range polys {
		switch c.Name {
		case p.TypeColumn:
			return "polymorphic: type discriminator"
		case p.IDColumn:
			return "polymorphic: id (target picked at runtime)"
		}
	}
	hasOverride := ov.Generator != "" || ov.Value != nil
	if !hasOverride && isSerialDefault(c.Default) {
		if keepDBManaged {
			return "generated: serial default (output mode)"
		}
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
