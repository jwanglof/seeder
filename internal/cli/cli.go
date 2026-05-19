package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mickamy/seeder/internal/exit"
	"github.com/mickamy/seeder/internal/insert"
	"github.com/mickamy/seeder/internal/introspect"
	"github.com/mickamy/seeder/internal/plan"
)

func Run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			PrintUsage(stdout)

			return exit.OK
		}
	}

	fs := flag.NewFlagSet("seeder", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { PrintUsage(stderr) }

	rows := fs.Int("rows", 1000, "rows per table")
	tablesArg := fs.String("tables", "", "comma-separated tables to include (default: all)")
	excludeArg := fs.String("exclude", "", "comma-separated tables to skip (mutually exclusive with --tables)")
	truncate := fs.Bool("truncate", false, "TRUNCATE before insert")
	seed := fs.Int64("seed", 0, "deterministic RNG seed (0 = time-based)")
	dryRun := fs.Bool("dry-run", false, "print plan, do not insert")

	reordered, err := reorderArgs(args, valueFlags)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: %v\n", err)

		return exit.Usage
	}

	if err := fs.Parse(reordered); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exit.OK
		}

		return exit.Usage
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "seeder: missing <dsn>")
		fmt.Fprintln(stderr, "Usage: seeder <dsn> [flags]")
		fmt.Fprintln(stderr, "Try:   seeder postgres://user:pass@localhost:5432/mydb")
		fmt.Fprintln(stderr, "Run 'seeder --help' for more info.")

		return exit.Usage
	}
	dsn := fs.Arg(0)

	if *rows < 0 {
		fmt.Fprintf(stderr, "seeder: --rows must be >= 0, got %d\n", *rows)

		return exit.Usage
	}

	if *seed < 0 {
		fmt.Fprintf(stderr, "seeder: --seed must be >= 0, got %d\n", *seed)

		return exit.Usage
	}

	if *tablesArg != "" && *excludeArg != "" {
		fmt.Fprintln(stderr, "seeder: --tables and --exclude are mutually exclusive")

		return exit.Usage
	}

	ctx := context.Background()

	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: introspect: %v\n", err)

		return exit.Error
	}

	switch {
	case *tablesArg != "":
		filtered, missing := includeTables(schema.Tables, splitTrim(*tablesArg))
		if len(missing) > 0 {
			fmt.Fprintf(stderr, "seeder: table not found: %s\n", strings.Join(missing, ", "))

			return exit.Usage
		}
		schema.Tables = filtered
	case *excludeArg != "":
		filtered, missing := excludeTables(schema.Tables, splitTrim(*excludeArg))
		if len(missing) > 0 {
			fmt.Fprintf(stderr, "seeder: table not found: %s\n", strings.Join(missing, ", "))

			return exit.Usage
		}
		schema.Tables = filtered
	}

	if len(schema.Tables) == 0 {
		fmt.Fprintln(stderr, "seeder: no tables to seed")

		return exit.Usage
	}

	if missing := orphanFKs(schema.Tables); len(missing) > 0 {
		fmt.Fprintf(stderr, "seeder: filtered tables have NOT NULL FK(s) to dropped tables:\n")
		for _, m := range missing {
			fmt.Fprintf(stderr, "  %s.%s -> %s.%s\n", m.FromTable, m.FromCol, m.ToTable, m.ToCol)
		}
		fmt.Fprintln(stderr, "Either include those parent tables or remove the filter.")

		return exit.Usage
	}

	order, err := plan.Build(schema.Tables)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: plan: %v\n", err)

		return exit.Error
	}

	fkCount := countFKs(schema.Tables)
	fmt.Fprintf(stdout, "seeder: %d table(s), %d FK(s)\n", len(order), fkCount)
	fmt.Fprintf(stdout, "order:  %s\n", strings.Join(order, " -> "))
	switch {
	case *dryRun:
		fmt.Fprintln(stdout, "mode:   dry-run (no INSERT)")
	case *truncate:
		fmt.Fprintln(stdout, "mode:   truncate + insert")
	default:
		fmt.Fprintln(stdout, "mode:   append")
	}

	opts := insert.Options{
		Rows:     *rows,
		Truncate: *truncate,
		DryRun:   *dryRun,
		Seed:     uint64(*seed),
	}

	start := time.Now()
	stats, err := insert.Run(ctx, dsn, schema, order, opts, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: %v\n", err)

		return exit.Error
	}
	elapsed := time.Since(start)

	var totalRows int64
	for _, s := range stats {
		totalRows += s.Rows
	}
	fmt.Fprintf(stdout, "done:   %d row(s) in %s\n", totalRows, elapsed.Truncate(time.Millisecond))

	return exit.OK
}

var valueFlags = map[string]bool{
	"rows":    true,
	"tables":  true,
	"exclude": true,
	"seed":    true,
}

// reorderArgs moves flags to the front so flag.Parse sees them even when
// the DSN comes first; stdlib flag.Parse stops at the first non-flag arg.
func reorderArgs(args []string, valueFlags map[string]bool) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	posArgs := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") {
			posArgs = append(posArgs, a)
			continue
		}
		flagArgs = append(flagArgs, a)
		if strings.ContainsRune(a, '=') {
			continue
		}
		name := strings.TrimLeft(a, "-")
		if valueFlags[name] {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag --%s needs a value", name)
			}
			flagArgs = append(flagArgs, args[i+1])
			i++
		}
	}

	return append(flagArgs, posArgs...), nil
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}

	return out
}

func includeTables(tables []introspect.Table, wanted []string) ([]introspect.Table, []string) {
	have := make(map[string]bool, len(tables))
	for _, t := range tables {
		have[t.Name] = true
	}
	want := make(map[string]bool, len(wanted))
	missing := make([]string, 0)
	for _, w := range wanted {
		if !have[w] {
			missing = append(missing, w)
			continue
		}
		want[w] = true
	}
	out := make([]introspect.Table, 0, len(want))
	for _, t := range tables {
		if want[t.Name] {
			out = append(out, t)
		}
	}

	return out, missing
}

func excludeTables(tables []introspect.Table, unwanted []string) ([]introspect.Table, []string) {
	have := make(map[string]bool, len(tables))
	for _, t := range tables {
		have[t.Name] = true
	}
	skip := make(map[string]bool, len(unwanted))
	missing := make([]string, 0)
	for _, w := range unwanted {
		if !have[w] {
			missing = append(missing, w)
			continue
		}
		skip[w] = true
	}
	out := make([]introspect.Table, 0, len(tables))
	for _, t := range tables {
		if !skip[t.Name] {
			out = append(out, t)
		}
	}

	return out, missing
}

type orphanFK struct {
	FromTable, FromCol string
	ToTable, ToCol     string
}

func orphanFKs(tables []introspect.Table) []orphanFK {
	in := make(map[string]bool, len(tables))
	for _, t := range tables {
		in[t.Name] = true
	}

	colNullable := func(t introspect.Table, col string) bool {
		for _, c := range t.Columns {
			if c.Name == col {
				return c.Nullable
			}
		}
		// Unknown column: treat as NOT NULL so the orphan FK surfaces
		// rather than silently passing the preflight.
		return false
	}

	var out []orphanFK
	for _, t := range tables {
		for _, fk := range t.ForeignKeys {
			if fk.ReferencedTable == t.Name {
				continue
			}
			if in[fk.ReferencedTable] {
				continue
			}
			for i, col := range fk.Columns {
				if colNullable(t, col) {
					continue
				}
				out = append(out, orphanFK{
					FromTable: t.Name,
					FromCol:   col,
					ToTable:   fk.ReferencedTable,
					ToCol:     fk.ReferencedColumns[i],
				})
			}
		}
	}

	return out
}

func countFKs(tables []introspect.Table) int {
	n := 0
	for _, t := range tables {
		n += len(t.ForeignKeys)
	}

	return n
}

func PrintUsage(w io.Writer) {
	fmt.Fprintln(w, "seeder — populate your database with realistic fake data, one command.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  seeder <dsn> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  seeder postgres://user:pass@localhost:5432/mydb")
	fmt.Fprintln(w, "  seeder postgres://...  --rows 10000")
	fmt.Fprintln(w, "  seeder postgres://...  --tables users,orders --rows 5000")
	fmt.Fprintln(w, "  seeder postgres://...  --exclude audit_log,migration_history")
	fmt.Fprintln(w, "  seeder postgres://...  --truncate --seed 42")
	fmt.Fprintln(w, "  seeder postgres://...  --dry-run")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --rows int       Rows per table (default: 1000)")
	fmt.Fprintln(w, "  --tables string  Comma-separated tables to include (default: all)")
	fmt.Fprintln(w, "  --exclude string Comma-separated tables to skip (cannot combine with --tables)")
	fmt.Fprintln(w, "  --truncate       TRUNCATE before insert (default: append)")
	fmt.Fprintln(w, "  --seed N         Deterministic RNG seed (>= 0; default: time-based)")
	fmt.Fprintln(w, "  --dry-run        Print plan, do not insert")
	fmt.Fprintln(w, "  --version, -v    Print seeder version")
	fmt.Fprintln(w, "  --help, -h       Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "More: https://github.com/mickamy/seeder")
}
