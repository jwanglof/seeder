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
	"no writable columns (all columns are identity or int-with-default); cannot seed in v0.1.0",
)

type Options struct {
	// Rows is the default row count; tables listed in RowsByTable override it.
	Rows        int
	RowsByTable map[string]int
	Truncate    bool
	DryRun      bool
	Verbose     bool
	// Seed is nil for a time-based RNG seed.
	Seed *uint64
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

func planColumns(t introspect.Table, faker *gofakeit.Faker) []colSpec {
	cols := make([]colSpec, 0, len(t.Columns))
	for _, c := range t.Columns {
		if c.IsIdentity {
			continue
		}
		if c.HasDefault && hasIntDefault(c.Kind) {
			continue
		}

		spec := colSpec{name: c.Name, nullable: c.Nullable}
		if fk, ok := findFK(t, c.Name); ok {
			spec.fk = fk
		} else {
			spec.gen = infer.Pick(faker, c)
		}
		cols = append(cols, spec)
	}

	return cols
}

// hasIntDefault returns true when an int-kind column is best left to the
// database default. The common case is a serial / IDENTITY column where the
// default is `nextval(...)`; we conservatively skip any int with a default
// (e.g., a `DEFAULT 0` counter) rather than parse the raw default expression.
// Parsing the expression so only true `nextval(...)` columns are skipped is
// planned for v0.2.0.
func hasIntDefault(kind introspect.Kind) bool {
	return kind == introspect.KindInt
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
		explainTable(t, out)
	}

	cols := planColumns(t, faker)
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
	for range rows {
		row := make([]any, len(cols))
		for j, c := range cols {
			if c.gen == nil {
				val, err := pickFK(faker, pool, c, t.Name)
				if err != nil {
					return Stats{Table: t.Name}, err
				}
				row[j] = val
				continue
			}
			row[j] = c.gen()
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

func joinColNames(cols []colSpec) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.name
	}

	return strings.Join(names, ", ")
}

func explainTable(t introspect.Table, out io.Writer) {
	fmt.Fprintf(out, "  %s\n", t.Name)
	for _, c := range t.Columns {
		fmt.Fprintf(out, "    %s\t%s\n", c.Name, explainColumn(t, c))
	}
}

func explainColumn(t introspect.Table, c introspect.Column) string {
	if c.IsIdentity {
		return "skip: identity"
	}
	if c.HasDefault && hasIntDefault(c.Kind) {
		return "skip: int with default"
	}
	if fk, ok := findFK(t, c.Name); ok {
		return fmt.Sprintf("fk: %s.%s", fk.table, fk.col)
	}

	return infer.Explain(c)
}
