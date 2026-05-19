package main

import (
	"fmt"
	"io"
	"os"

	"github.com/mickamy/seeder/internal/cli"
	"github.com/mickamy/seeder/internal/exit"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)

		return exit.OK
	}

	switch args[0] {
	case "--version", "-v", "version":
		fmt.Fprintf(stdout, "seeder %s\n", version)

		return exit.OK
	case "--help", "-h", "help":
		printUsage(stdout)

		return exit.OK
	}

	return cli.Run(args, stdout, stderr)
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
