package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mickamy/seeder/internal/config"
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

	configPath := fs.String("config", "", "path to seeder.yaml (default: auto-detect in CWD)")
	dryRun := fs.Bool("dry-run", false, "print plan, do not insert")
	excludeArg := fs.String("exclude", "", "comma-separated tables to skip (mutually exclusive with --tables)")
	rows := fs.Int("rows", 1000, "rows per table")
	seed := fs.Int64("seed", 0, "deterministic RNG seed (default: time-based when omitted)")
	tablesArg := fs.String("tables", "", "comma-separated tables to include (default: all)")
	truncate := fs.Bool("truncate", false, "TRUNCATE before insert")
	verbose := fs.Bool("verbose", false, "print per-column inference decisions")

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
		printMissingDSN(stderr)

		return exit.Usage
	}
	dsn := fs.Arg(0)

	if msg := validateFlags(*rows, *seed, *tablesArg, *excludeArg); msg != "" {
		fmt.Fprintln(stderr, "seeder: "+msg)

		return exit.Usage
	}

	set := flagSet(fs)

	cfg, err := configAt(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: %v\n", err)

		return exit.Usage
	}

	ctx := context.Background()

	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: introspect: %v\n", err)

		return exit.Error
	}

	if unknown := unknownConfigTables(cfg, schema); len(unknown) > 0 {
		fmt.Fprintf(stderr, "seeder: seeder.yaml references unknown table(s): %s\n", strings.Join(unknown, ", "))

		return exit.Usage
	}

	schema, missing := applyTableFilters(schema, *tablesArg, *excludeArg, cfg)
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "seeder: table not found: %s\n", strings.Join(missing, ", "))

		return exit.Usage
	}

	if len(schema.Tables) == 0 {
		fmt.Fprintln(stderr, "seeder: no tables to seed")

		return exit.Usage
	}

	if orphans := orphanFKs(schema.Tables); len(orphans) > 0 {
		printOrphanFKs(stderr, orphans)

		return exit.Usage
	}

	order, err := plan.Build(schema.Tables)
	if err != nil {
		fmt.Fprintf(stderr, "seeder: plan: %v\n", err)

		return exit.Error
	}

	opts := buildInsertOptions(*rows, *truncate, *seed, *dryRun, *verbose, set, cfg)
	printHeader(stdout, order, countFKs(schema.Tables), opts)

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

func flagSet(fs *flag.FlagSet) map[string]bool {
	out := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { out[f.Name] = true })

	return out
}

func validateFlags(rows int, seed int64, tablesArg, excludeArg string) string {
	switch {
	case rows < 0:
		return fmt.Sprintf("--rows must be >= 0, got %d", rows)
	case seed < 0:
		return fmt.Sprintf("--seed must be >= 0, got %d", seed)
	case tablesArg != "" && excludeArg != "":
		return "--tables and --exclude are mutually exclusive"
	}

	return ""
}

func applyTableFilters(
	schema introspect.Schema, tablesArg, excludeArg string, cfg config.Config,
) (introspect.Schema, []string) {
	switch {
	case tablesArg != "":
		filtered, missing := includeTables(schema.Tables, splitTrim(tablesArg))
		schema.Tables = filtered

		return schema, missing
	case excludeArg != "":
		filtered, missing := excludeTables(schema.Tables, splitTrim(excludeArg))
		schema.Tables = filtered

		return schema, missing
	}
	if yamlExclude := yamlExcludeList(cfg); len(yamlExclude) > 0 {
		filtered, _ := excludeTables(schema.Tables, yamlExclude)
		schema.Tables = filtered
	}

	return schema, nil
}

func buildInsertOptions(
	rows int, truncate bool, seed int64, dryRun, verbose bool,
	set map[string]bool, cfg config.Config,
) insert.Options {
	defaultRows := rows
	if !set["rows"] && cfg.Rows != nil {
		defaultRows = *cfg.Rows
	}

	var rowsByTable map[string]int
	if !set["rows"] {
		rowsByTable = perTableRows(cfg)
	}

	effectiveTruncate := truncate
	if !set["truncate"] && cfg.Truncate != nil {
		effectiveTruncate = *cfg.Truncate
	}

	opts := insert.Options{
		Rows:        defaultRows,
		RowsByTable: rowsByTable,
		Truncate:    effectiveTruncate,
		DryRun:      dryRun,
		Verbose:     verbose,
	}
	switch {
	case set["seed"]:
		s := uint64(seed) //nolint:gosec // seed is validated >= 0 in validateFlags
		opts.Seed = &s
	case cfg.Seed != nil:
		s := *cfg.Seed
		opts.Seed = &s
	}

	return opts
}

func printMissingDSN(w io.Writer) {
	fmt.Fprintln(w, "seeder: missing <dsn>")
	fmt.Fprintln(w, "Usage: seeder <dsn> [flags]")
	fmt.Fprintln(w, "Try:   seeder postgres://user:pass@localhost:5432/mydb")
	fmt.Fprintln(w, "Run 'seeder --help' for more info.")
}

func printOrphanFKs(w io.Writer, orphans []orphanFK) {
	fmt.Fprintln(w, "seeder: filtered tables have NOT NULL FK(s) to dropped tables:")
	for _, m := range orphans {
		fmt.Fprintf(w, "  %s.%s -> %s.%s\n", m.FromTable, m.FromCol, m.ToTable, m.ToCol)
	}
	fmt.Fprintln(w, "Either include those parent tables or remove the filter.")
}

func printHeader(w io.Writer, order []string, fkCount int, opts insert.Options) {
	fmt.Fprintf(w, "seeder: %d table(s), %d FK(s)\n", len(order), fkCount)
	fmt.Fprintf(w, "order:  %s\n", strings.Join(order, " -> "))
	switch {
	case opts.DryRun:
		fmt.Fprintln(w, "mode:   dry-run (no INSERT)")
	case opts.Truncate:
		fmt.Fprintln(w, "mode:   truncate + insert")
	default:
		fmt.Fprintln(w, "mode:   append")
	}
}

var valueFlags = map[string]bool{
	"rows":    true,
	"tables":  true,
	"exclude": true,
	"seed":    true,
	"config":  true,
}

// configAt loads the config at path; an empty path auto-detects seeder.yaml in the CWD.
//
//nolint:wrapcheck // config package already wraps errors with a seeder.yaml: prefix
func configAt(path string) (config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return config.Config{}, fmt.Errorf("cwd: %w", err)
	}
	cfg, _, err := config.AutoDetect(cwd)

	return cfg, err
}

func perTableRows(cfg config.Config) map[string]int {
	if len(cfg.Tables) == 0 {
		return nil
	}
	out := make(map[string]int, len(cfg.Tables))
	for name, tc := range cfg.Tables {
		if tc.Rows != nil {
			out[name] = *tc.Rows
		}
	}
	if len(out) == 0 {
		return nil
	}

	return out
}

func yamlExcludeList(cfg config.Config) []string {
	var out []string
	for name, tc := range cfg.Tables {
		if tc.Exclude {
			out = append(out, name)
		}
	}

	return out
}

func unknownConfigTables(cfg config.Config, schema introspect.Schema) []string {
	if len(cfg.Tables) == 0 {
		return nil
	}
	known := make(map[string]bool, len(schema.Tables))
	for _, t := range schema.Tables {
		known[t.Name] = true
	}
	var out []string
	for name := range cfg.Tables {
		if !known[name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)

	return out
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
	fmt.Fprintln(w, "  --config <file>  Path to seeder.yaml (default: auto-detect ./seeder.yaml)")
	fmt.Fprintln(w, "  --dry-run        Print plan, do not insert")
	fmt.Fprintln(w, "  --exclude string Comma-separated tables to skip (cannot combine with --tables)")
	fmt.Fprintln(w, "  --rows int       Rows per table (default: 1000; overrides yaml when set)")
	fmt.Fprintln(w, "  --seed N         Deterministic RNG seed (>= 0; default: time-based)")
	fmt.Fprintln(w, "  --tables string  Comma-separated tables to include (default: all)")
	fmt.Fprintln(w, "  --truncate       TRUNCATE before insert (default: append)")
	fmt.Fprintln(w, "  --verbose        Print per-column inference decisions")
	fmt.Fprintln(w, "  --version, -v    Print seeder version")
	fmt.Fprintln(w, "  --help, -h       Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "More: https://github.com/mickamy/seeder")
}
