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
		cli.PrintUsage(stdout)

		return exit.OK
	}

	for _, a := range args {
		switch a {
		case "--version", "-v":
			fmt.Fprintf(stdout, "seeder %s\n", version)

			return exit.OK
		case "--help", "-h":
			cli.PrintUsage(stdout)

			return exit.OK
		}
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "seeder %s\n", version)

		return exit.OK
	case "help":
		cli.PrintUsage(stdout)

		return exit.OK
	}

	return cli.Run(args, stdout, stderr)
}
