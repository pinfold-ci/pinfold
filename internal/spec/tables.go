package spec

// SupportedVersion is the only spec version this compiler reads.
const SupportedVersion = 0

// Triggers lists the events v0.1 compiles and the keys each accepts.
var Triggers = map[string][]string{
	"push":                        {"branches", "tags", "paths"},
	"schedule":                    {"cron"},
	"manual":                      {},
	"issue_comment":               {"types"},
	"issues":                      {"types"},
	"pull_request_target":         {"types", "branches"},
	"pull_request_review":         {"types"},
	"pull_request_review_comment": {"types"},
	"discussion":                  {"types"},
	"discussion_comment":          {"types"},
}

// unsupportedTriggers explains events Pinfold knows about but doesn't compile.
var unsupportedTriggers = map[string]string{
	"pull_request":               "untrusted pull_request builds arrive in v0.2 (handoff); keep this workflow hand-written for now",
	"workflow_dispatch":          `use "manual"`,
	"workflow_run":               "user-written workflow_run workflows aren't compiled in v0.1",
	"workflow_call":              "reusable workflows aren't supported in v0.1",
	"pull_request_review_thread": "not supported in v0.1",
}

// PermissionLevels gives the accepted levels for each scope of the workflow
// "permissions:" key (GitHub docs, workflow syntax, checked 2026-10-30).
// This table is versioned data: update it with the docs, not ad hoc.
var PermissionLevels = map[string][]string{
	"actions":              {"read", "write", "none"},
	"artifact-metadata":    {"read", "write", "none"},
	"attestations":         {"read", "write", "none"},
	"checks":               {"read", "write", "none"},
	"code-quality":         {"read", "write", "none"},
	"contents":             {"read", "write", "none"},
	"copilot-requests":     {"write", "none"},
	"deployments":          {"read", "write", "none"},
	"discussions":          {"read", "write", "none"},
	"id-token":             {"write", "none"},
	"issues":               {"read", "write", "none"},
	"packages":             {"read", "write", "none"},
	"pages":                {"read", "write", "none"},
	"pull-requests":        {"read", "write", "none"},
	"security-events":      {"read", "write", "none"},
	"statuses":             {"read", "write", "none"},
	"vulnerability-alerts": {"read", "none"},
}

// Contexts lists the context names a value may reference. Taint (M3) decides
// where each may go; this list only rejects names that don't exist.
var Contexts = []string{
	"actor", "triggering_actor",
	"commit.sha",
	"comment.body", "comment.id",
	"event.name",
	"issue.number", "issue.title", "issue.body",
	"pr.number", "pr.title", "pr.body", "pr.author_association", "pr.head.ref", "pr.head.sha", "pr.base.ref",
	"ref.name",
	"repo.full_name", "repo.default_branch",
	"run.id", "run.attempt",
}

// Effect field tables: which keys each effect accepts, and which it requires.
var effectFields = map[string]struct{ allowed, required []string }{
	"comment":       {[]string{"target", "body", "sticky"}, []string{"target", "body"}},
	"add_labels":    {[]string{"target", "labels", "allowed", "max"}, []string{"target", "labels", "allowed"}},
	"remove_labels": {[]string{"target", "labels", "allowed", "max"}, []string{"target", "labels"}},
	"commit_status": {[]string{"sha", "state", "context", "description"}, []string{"sha", "state", "context"}},
	"release":       {[]string{"tag", "files", "generate_notes", "draft", "prerelease"}, []string{"tag"}},
}

var commitStates = []string{"error", "failure", "pending", "success"}

var statusValues = []string{"always", "failure", "not_cancelled", "success"}

var associations = []string{"COLLABORATOR", "CONTRIBUTOR", "FIRST_TIMER", "FIRST_TIME_CONTRIBUTOR", "MANNEQUIN", "MEMBER", "NONE", "OWNER"}

// Key tables (also used by the JSON Schema consistency test).
var (
	topKeys       = []string{"version", "workflows"}
	workflowKeys  = []string{"on", "jobs", "unsafe_run"}
	jobKeys       = []string{"runner", "needs", "if", "permissions", "environment", "timeout-minutes", "concurrency", "steps", "produces", "consumes"}
	stepCommon    = []string{"name"}
	stepKinds     = []string{"checkout", "run", "uses", "effect"}
	conditionKeys = []string{"all", "any", "event", "branch", "tag", "label", "comment", "maintainer", "association", "actor", "status"}
)
