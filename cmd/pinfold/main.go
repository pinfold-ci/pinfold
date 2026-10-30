// Command pinfold compiles a pinfold.yml spec into secure GitHub Actions workflows.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/version"
)

const usage = `pinfold compiles pinfold.yml into secure GitHub Actions workflows.

Usage:
  pinfold <command> [arguments]

Commands:
  explain    explain a diagnostic code, e.g. "pinfold explain PF110"
  version    print the compiler version
  help       show this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintf(stdout, "pinfold %s\n", version.Version)
		return 0
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "pinfold: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func runExplain(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: pinfold explain <code>   (for example: pinfold explain PF110)")
		return 2
	}
	page, err := codes.Explain(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "pinfold explain: %v\n", err)
		return 2
	}
	fmt.Fprint(stdout, page)
	return 0
}
