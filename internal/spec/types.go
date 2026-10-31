// Package spec defines the pinfold.yml source format and decodes it from a
// YAML node tree, reporting every problem as a positioned diagnostic.
package spec

import "github.com/pinfold-ci/pinfold/internal/diag"

// Spec is a whole pinfold.yml.
type Spec struct {
	Version   int
	Workflows []*Workflow // in source order
}

// Workflow becomes one .github/workflows/pinfold-<name>.yml.
type Workflow struct {
	Name      string
	Span      diag.Span
	On        []*Trigger // in source order
	Jobs      []*Job     // in source order
	UnsafeRun bool
}

// Trigger is one event under "on:".
type Trigger struct {
	Event    string
	Span     diag.Span
	Branches []string
	Tags     []string
	Paths    []string
	Types    []string
	Crons    []string // schedule only
}

// Job is one job in a workflow.
type Job struct {
	Name            string
	Span            diag.Span
	Runner          string
	EphemeralRunner bool
	Needs           []Ref
	If              *Condition
	Permissions     []Permission // in source order
	Environment     string
	TimeoutMinutes  int
	Concurrency     *Concurrency
	Steps           []*Step
	Produces        []Artifact
	Consumes        []Consume
}

// Ref is a reference to another job, with the position of the reference.
type Ref struct {
	Name string
	Span diag.Span
}

// Permission is one declared token scope.
type Permission struct {
	Scope string
	Level string // read, write or none
	Span  diag.Span
}

// Concurrency is a job's concurrency group.
type Concurrency struct {
	Group            string
	CancelInProgress bool
}

// Artifact is a directory a job produces for later jobs.
type Artifact struct {
	Name string
	Path string
	Span diag.Span
}

// Consume is an artifact a job downloads from an earlier job.
type Consume struct {
	Name string
	From Ref
	To   string
	Span diag.Span
}

// StepKind says which of the four step forms a step uses.
type StepKind string

const (
	StepCheckout StepKind = "checkout"
	StepRun      StepKind = "run"
	StepUses     StepKind = "uses"
	StepEffect   StepKind = "effect"
)

// Step is one step in a job.
type Step struct {
	Kind StepKind
	Span diag.Span
	Name string

	Run string           // run
	Env map[string]Value // run

	Uses string           // uses
	With map[string]Value // uses

	Effect *Effect // effect
}

// ValueKind says where a value comes from.
type ValueKind string

const (
	Literal ValueKind = "literal"
	Context ValueKind = "context"
	Secret  ValueKind = "secret"
	Var     ValueKind = "var"
	Output  ValueKind = "output"
)

// Value is a typed value: a literal, or a reference the compiler resolves.
type Value struct {
	Kind ValueKind
	Text string // the literal text, or the referenced name
	Span diag.Span
}

// Effect is a compiler-generated operation (design §5, "Effects").
type Effect struct {
	Name string
	Span diag.Span

	Target *Value // comment, add_labels, remove_labels

	Body   string // comment
	Sticky string // comment

	Labels          []string // add_labels, remove_labels: literal labels
	FromCommentArgs bool     // add_labels, remove_labels: labels come from the comment text
	Allowed         []string // add_labels, remove_labels
	Max             int      // add_labels, remove_labels

	SHA         *Value // commit_status
	State       string // commit_status
	Context     string // commit_status
	Description string // commit_status

	Tag           *Value   // release
	Files         []string // release
	GenerateNotes bool     // release
	Draft         bool     // release
	Prerelease    bool     // release
}

// Condition is a structured "if:". Exactly one field group is set.
type Condition struct {
	Span diag.Span
	All  []*Condition
	Any  []*Condition

	Kind   string   // the leaf condition's key, e.g. "event" or "maintainer"
	Values []string // event, branch, tag, label, association, actor, status, comment.starts_with
	Flag   bool     // maintainer
}
