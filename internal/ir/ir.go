// Package ir is Pinfold's platform-neutral intermediate representation. The
// core owns every node's zone and capabilities; adapters only translate them.
package ir

import (
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/spec"
	"slices"
)

// Value, Condition and Effect are shared with the source format: they are
// already typed and platform-neutral.
type (
	Value     = spec.Value
	Condition = spec.Condition
	Effect    = spec.Effect
)

// Program is a whole compiled spec.
type Program struct {
	Workflows []*Workflow
}

// TriggerClass is the trust class of a workflow's triggers (design §4).
type TriggerClass string

const (
	Trusted    TriggerClass = "trusted"    // push, schedule, manual
	Privileged TriggerClass = "privileged" // everything else: default-deny
)

// Workflow is one generated workflow file.
type Workflow struct {
	ID        string
	Span      diag.Span
	Class     TriggerClass
	Triggers  []*spec.Trigger
	Jobs      []*Job
	UnsafeRun bool
}

// Zone is a job's trust zone (design §4, "Job zones").
type Zone string

const (
	ZoneUnprivileged Zone = "unprivileged"
	ZoneFeeder       Zone = "feeder"     // unprivileged, but a privileged job consumes its artifacts or outputs
	ZonePrivileged   Zone = "privileged" // holds credentials or runs under a privileged trigger
)

// Access is a capability level.
type Access string

const (
	Read  Access = "read"
	Write Access = "write"
)

// Capability is an abstract permission (design §8); adapters map it to their platform.
type Capability struct {
	Resource string // e.g. "repo.contents", "issues", "oidc"
	Access   Access
	Source   string // "declared", or the generated feature that needs it
}

// Job is one job.
type Job struct {
	ID              string
	Span            diag.Span
	Name            string
	Runner          string
	EphemeralRunner bool // the spec asserts a fresh machine per job (security audit D1)
	Needs           []string
	If              *Condition
	Zone            Zone
	Capabilities    []Capability // sorted by resource
	Environment     string
	TimeoutMinutes  int
	Concurrency     *spec.Concurrency
	Steps           []*Step
	Produces        []spec.Artifact
	Consumes        []spec.Consume
}

// StepKind is the kind of a step.
type StepKind string

const (
	Checkout StepKind = "checkout"
	Run      StepKind = "run"
	Uses     StepKind = "uses"
	DoEffect StepKind = "effect"
)

// Step is one step. Exactly the fields for its Kind are set.
type Step struct {
	ID     string
	Span   diag.Span
	Kind   StepKind
	Name   string
	Run    string
	Env    map[string]Value
	Uses   string
	With   map[string]Value
	Effect *Effect
}

// GitHubHostedRunners are the standard GitHub-hosted runner labels, which
// give every job a fresh virtual machine. Source:
// https://docs.github.com/en/actions/reference/runners/github-hosted-runners
// (checked 2026-10-31). Trust zones depend on that (design §2): on any other
// runner a job in a feeder or privileged zone needs ephemeral_runner: true.
var GitHubHostedRunners = []string{
	"ubuntu-latest", "ubuntu-slim", "ubuntu-26.04", "ubuntu-24.04", "ubuntu-22.04",
	"ubuntu-26.04-arm", "ubuntu-24.04-arm", "ubuntu-22.04-arm",
	"windows-latest", "windows-2025", "windows-2025-vs2026", "windows-2022",
	"windows-11-arm", "windows-11-vs2026-arm",
	"macos-latest", "macos-26", "macos-15", "macos-14", "macos-26-intel", "macos-15-intel", "xcode-27",
}

// FreshRunner reports whether j runs on a machine that is new for this job:
// a standard GitHub-hosted label (exact case), or one the spec declares
// ephemeral.
func FreshRunner(j *Job) bool {
	return j.EphemeralRunner || slices.Contains(GitHubHostedRunners, j.Runner)
}
