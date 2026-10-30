// Command pinfold compiles a pinfold.yml spec into secure GitHub Actions workflows.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
	"github.com/pinfold-ci/pinfold/internal/version"
)

const usage = `pinfold compiles pinfold.yml into secure GitHub Actions workflows.

Usage:
  pinfold <command> [arguments]

Commands:
  validate   check pinfold.yml and report problems
  schema     print the JSON Schema for pinfold.yml (for editors)
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
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "schema":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: pinfold schema")
			return 2
		}
		if _, err := stdout.Write(spec.Schema); err != nil {
			return 2
		}
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

// runValidate checks a spec. Exit 0: no errors (warnings allowed); 1: errors; 2: usage.
func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text or json")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: pinfold validate [--format text|json] [pinfold.yml]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*format != "text" && *format != "json") || fs.NArg() > 1 {
		fs.Usage()
		return 2
	}
	path := "pinfold.yml"
	if fs.NArg() == 1 {
		path = fs.Arg(0)
	}
	diags := validate(path)
	var err error
	if *format == "json" {
		err = diags.WriteJSON(stdout)
	} else {
		err = diags.WriteText(stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "pinfold validate: %v\n", err)
		return 2
	}
	if diags.HasErrors() {
		return 1
	}
	return 0
}

func validate(path string) diag.List {
	root, diags := load.ReadFile(path)
	if diags.HasErrors() {
		return diags
	}
	_, diags = spec.Decode(path, root)
	return diags
}
