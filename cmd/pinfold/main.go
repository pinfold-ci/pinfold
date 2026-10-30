// Command pinfold compiles a pinfold.yml spec into secure GitHub Actions workflows.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/pinfold-ci/pinfold/internal/version"
)

const usage = `pinfold compiles pinfold.yml into secure GitHub Actions workflows.

Usage:
  pinfold <command> [arguments]

Commands:
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
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "pinfold: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
