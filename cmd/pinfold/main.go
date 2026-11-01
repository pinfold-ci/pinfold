// Command pinfold compiles a pinfold.yml spec into secure GitHub Actions workflows.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/compile"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/initcmd"
	"github.com/pinfold-ci/pinfold/internal/spec"
	"github.com/pinfold-ci/pinfold/internal/version"
)

const usage = `pinfold compiles pinfold.yml into secure GitHub Actions workflows.

Usage:
  pinfold <command> [arguments]

Commands:
  init       create a starter pinfold.yml and print setup steps
  compile    compile pinfold.yml into .github/workflows/pinfold-*.yml
  ir         print the intermediate representation as JSON
  validate   check pinfold.yml and report problems
  schema     print the JSON Schema for pinfold.yml (for editors)
  explain    explain a diagnostic code, e.g. "pinfold explain PF110";
             with no code, list every code
  version    print the compiler version
  help       show this help

Getting started:
  1. pinfold init        write a starter pinfold.yml in the repository root
  2. edit pinfold.yml    one spec holds every workflow ("workflows:")
  3. pinfold validate    errors come as file:line:col, a PFxxx code and a fix hint
  4. pinfold compile     writes .github/workflows/pinfold-<name>.yml per workflow
  5. commit pinfold.yml and the generated files together

  Never edit the generated files: change pinfold.yml and compile again.
  "pinfold compile --check" exits 2 when they are out of date (use it in CI).

How a spec works:
  Jobs that hold credentials (write permissions, id-token, secrets, an
  environment, or any trigger other than push, schedule and manual) can't
  run your own scripts. Build in an unprivileged job with "produces:", then
  "consumes:" the artifact in the privileged job and use effects (comment,
  add_labels, remove_labels, commit_status, release) or reviewed actions.
  Event data (titles, comments, branch names) only reaches scripts through
  env: names that start with T_.

  "pinfold schema" prints a JSON Schema that describes every key, for
  editors and coding agents.
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
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "ir":
		return runIR(args[1:], stdout, stderr)
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
	switch len(args) {
	case 0:
		listCodes(stdout)
		return 0
	case 1:
		page, err := codes.Explain(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "pinfold explain: %v\n", err)
			return 2
		}
		fmt.Fprint(stdout, page)
		return 0
	default:
		fmt.Fprintln(stderr, "usage: pinfold explain [code]   (for example: pinfold explain PF110)")
		return 2
	}
}

// listCodes prints every diagnostic code with its severity and title.
func listCodes(stdout io.Writer) {
	for _, c := range codes.All() {
		e, _ := codes.Lookup(c)
		fmt.Fprintf(stdout, "%s  %-7s  %s\n", c, e.Severity, e.Title)
	}
	fmt.Fprintln(stdout, "\nRun \"pinfold explain <code>\" for the full page, e.g. pinfold explain PF301.")
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
	_, _, diags := compile.Front(path)
	return diags
}

// runCompile writes the generated workflows next to the spec, or with --check
// compares them. Exit 0: done or up to date; 1: errors; 2: usage, or drift with --check.
func runCompile(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "don't write; exit 2 if the generated files are missing or differ")
	fs.Usage = func() { fmt.Fprintln(stderr, "usage: pinfold compile [--check] [pinfold.yml]") }
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		if err == nil {
			fs.Usage()
		}
		return 2
	}
	path := "pinfold.yml"
	if fs.NArg() == 1 {
		path = fs.Arg(0)
	}
	res, diags := compile.Run(path)
	if err := diags.WriteText(stderr); err != nil {
		return 2
	}
	if res == nil {
		return 1
	}
	dir := filepath.Dir(path)
	if *check {
		drift, err := compile.Drift(dir, res)
		if err != nil {
			fmt.Fprintf(stderr, "pinfold compile: %v\n", err)
			return 1
		}
		if len(drift) > 0 {
			for _, d := range drift {
				fmt.Fprintf(stdout, "out of date: %s\n", d)
			}
			fmt.Fprintln(stdout, `run "pinfold compile" and commit the result`)
			return 2
		}
		return 0
	}
	if err := compile.Write(dir, res); err != nil {
		fmt.Fprintf(stderr, "pinfold compile: %v\n", err)
		return 1
	}
	for _, f := range res.Files {
		fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(dir, f.Path))
	}
	return 0
}

// runIR prints the IR as JSON.
func runIR(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: pinfold ir [pinfold.yml]")
		return 2
	}
	path := "pinfold.yml"
	if len(args) == 1 {
		path = args[0]
	}
	p, _, diags := compile.Front(path)
	if err := diags.WriteText(stderr); err != nil {
		return 2
	}
	if p == nil {
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return 2
	}
	return 0
}

func runInit(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: pinfold init [pinfold.yml]")
		return 2
	}
	path := "pinfold.yml"
	if len(args) == 1 {
		path = args[0]
	}
	if err := initcmd.Run(path, stdout); err != nil {
		fmt.Fprintf(stderr, "pinfold init: %v\n", err)
		return 1
	}
	return 0
}
