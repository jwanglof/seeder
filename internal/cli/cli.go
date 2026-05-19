package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/mickamy/seeder/internal/exit"
)

func Run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			printUsage(stdout)

			return exit.OK
		}
	}

	fs := flag.NewFlagSet("seeder", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }

	rows := fs.Int("rows", 1000, "rows per table")
	tables := fs.String("tables", "", "comma-separated tables (default: all non-system)")
	truncate := fs.Bool("truncate", false, "TRUNCATE before insert")
	seed := fs.Int64("seed", 0, "deterministic RNG seed (default: time-based)")
	dryRun := fs.Bool("dry-run", false, "print plan, do not insert")

	if err := fs.Parse(args); err != nil {
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
	_ = dsn
	_ = rows
	_ = tables
	_ = truncate
	_ = seed
	_ = dryRun

	fmt.Fprintln(stderr, "seeder: not yet implemented")

	return exit.NotImplemented
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "seeder — populate your database with realistic fake data, one command.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  seeder <dsn> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  seeder postgres://user:pass@localhost:5432/mydb")
	fmt.Fprintln(w, "  seeder postgres://...  --rows 10000")
	fmt.Fprintln(w, "  seeder postgres://...  --tables users,orders --rows 5000")
	fmt.Fprintln(w, "  seeder postgres://...  --truncate --seed 42")
	fmt.Fprintln(w, "  seeder postgres://...  --dry-run")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --rows int       Rows per table (default: 1000)")
	fmt.Fprintln(w, "  --tables string  Comma-separated tables (default: all non-system)")
	fmt.Fprintln(w, "  --truncate       TRUNCATE before insert (default: append)")
	fmt.Fprintln(w, "  --seed int       Deterministic RNG seed (default: time-based)")
	fmt.Fprintln(w, "  --dry-run        Print plan, do not insert")
	fmt.Fprintln(w, "  --version, -v    Print seeder version")
	fmt.Fprintln(w, "  --help, -h       Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "More: https://github.com/mickamy/seeder")
}
