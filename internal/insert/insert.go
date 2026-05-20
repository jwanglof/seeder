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
	Truncate    bool
	DryRun      bool
	Verbose     bool
	// Seed is nil for a time-based RNG seed.
	Seed            *uint64
	Locale          infer.Locale
	ColumnOverrides map[string]map[string]ColumnOverride
}

type ColumnOverride struct {
	Generator string
	Value     any
}

type Stats struct {
	Table string
	Rows  int64
	Took  time.Duration
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

	pool := make(map[string]map[string][]any)
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

type colSpec struct {
	name     string
	nullable bool
	// gen is non-nil for a column whose value seeder generates itself.
	// gen is nil for FK columns; in that case `fk` is meaningful.
	gen generator.Func
	fk  fkSpec
}

type fkSpec struct {
	table string
	col   string
}

func planColumns(
	t introspect.Table, faker *gofakeit.Faker,
	locale infer.Locale, overrides map[string]ColumnOverride,
) ([]colSpec, error) {
	cols := make([]colSpec, 0, len(t.Columns))
	for _, c := range t.Columns {
		if c.IsIdentity {
			continue
		}
		// FK columns always go through the FK pool, even when they have a
		// default or a yaml override (the override is intentionally ignored).
		if fk, ok := findFK(t, c.Name); ok {
			cols = append(cols, colSpec{name: c.Name, nullable: c.Nullable, fk: fk})
			continue
		}
		ov := overrides[c.Name]
		hasOverride := ov.Generator != "" || ov.Value != nil
		// A yaml override wins over the serial-default skip; the user
		// asked for a specific value/generator, so honor it.
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

func findFK(t introspect.Table, column string) (fkSpec, bool) {
	for _, fk := range t.ForeignKeys {
		for i, c := range fk.Columns {
			if c == column {
				return fkSpec{table: fk.ReferencedTable, col: fk.ReferencedColumns[i]}, true
			}
		}
	}

	return fkSpec{}, false
}

func insertTable(
	ctx context.Context,
	drv Driver,
	t introspect.Table,
	opts Options,
	faker *gofakeit.Faker,
	pool map[string]map[string][]any,
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

	data := make([][]any, 0, rows)
	// inBatch lets a later row's self-FK reference an earlier row's PK before
	// the parent table is flushed.
	inBatch := make(map[string][]any)
	for range rows {
		row := make([]any, len(cols))
		for j, c := range cols {
			if c.gen != nil {
				row[j] = c.gen()
			}
		}
		for j, c := range cols {
			if c.gen != nil {
				continue
			}
			if c.fk.table == t.Name {
				val, err := pickSelfFK(faker, pool, inBatch, row, cols, c, t.Name)
				if err != nil {
					return Stats{Table: t.Name}, err
				}
				row[j] = val
				continue
			}
			val, err := pickFK(faker, pool, c, t.Name)
			if err != nil {
				return Stats{Table: t.Name}, err
			}
			row[j] = val
		}
		for j, c := range cols {
			if c.gen != nil {
				inBatch[c.name] = append(inBatch[c.name], row[j])
			}
		}
		data = append(data, row)
	}

	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.name
	}

	start := time.Now()
	n, err := drv.BulkInsert(ctx, t.Name, colNames, data)
	if err != nil {
		return Stats{Table: t.Name}, fmt.Errorf("bulk insert: %w", err)
	}
	took := time.Since(start)

	if len(poolCols) > 0 {
		vals, err := drv.ColumnValues(ctx, t.Name, poolCols)
		if err != nil {
			return Stats{Table: t.Name, Rows: n, Took: took}, fmt.Errorf("column values: %w", err)
		}
		pool[t.Name] = vals
	}

	fmt.Fprintf(out, "  %s\t%d rows (%s)\n", t.Name, n, took.Truncate(time.Microsecond))

	return Stats{Table: t.Name, Rows: n, Took: took}, nil
}

func pickFK(faker *gofakeit.Faker, pool map[string]map[string][]any, c colSpec, tableName string) (any, error) {
	vals := pool[c.fk.table][c.fk.col]
	if len(vals) == 0 {
		if !c.nullable {
			return nil, fmt.Errorf("FK target %s.%s has no rows but %s.%s is NOT NULL", c.fk.table, c.fk.col, tableName, c.name)
		}

		return nil, nil //nolint:nilnil // intentional NULL for a nullable FK with empty parent pool
	}

	return vals[faker.IntRange(0, len(vals)-1)], nil
}

func pickSelfFK(
	faker *gofakeit.Faker,
	pool map[string]map[string][]any,
	inBatch map[string][]any,
	row []any,
	cols []colSpec,
	c colSpec,
	tableName string,
) (any, error) {
	poolVals := pool[c.fk.table][c.fk.col]
	inBatchVals := inBatch[c.fk.col]
	total := len(poolVals) + len(inBatchVals)
	if total > 0 {
		idx := faker.IntRange(0, total-1)
		if idx < len(poolVals) {
			return poolVals[idx], nil
		}

		return inBatchVals[idx-len(poolVals)], nil
	}

	if c.nullable {
		return nil, nil //nolint:nilnil // intentional NULL for a nullable self-FK on the first batch row
	}

	// Row 0 self-loop: only works when the referenced PK is seeder-generated;
	// a DB-generated PK (serial / AUTO_INCREMENT) is not yet known here.
	if pkVal, ok := lookupOwnPK(row, cols, c.fk.col); ok {
		return pkVal, nil
	}

	return nil, fmt.Errorf(
		"self-FK %s.%s is NOT NULL but no seeded values are available and %s.%s is DB-generated",
		tableName, c.name, c.fk.table, c.fk.col,
	)
}

func lookupOwnPK(row []any, cols []colSpec, pkCol string) (any, bool) {
	for j, c := range cols {
		if c.name == pkCol && c.gen != nil {
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
	if fk, ok := findFK(t, c.Name); ok {
		return fmt.Sprintf("fk: %s.%s", fk.table, fk.col)
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
