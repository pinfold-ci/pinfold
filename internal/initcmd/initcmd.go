// Package initcmd implements "pinfold init": a starter spec plus the repository
// settings Pinfold's best-effort protections depend on (design §2 and §7).
package initcmd

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

//go:embed starter.yml
var Starter []byte

// Instructions is printed after the starter spec is written.
const Instructions = `Created %s.

Next steps:
  1. Edit %s, then run "pinfold validate".

Repository settings (Pinfold can't set these for you; see design §2, best-effort):
  2. Make the status check "pinfold/verify" required on your default branch,
     with GitHub Actions as the expected source. (The generated pinfold-verify
     workflows post it from milestone M4 onwards.)
  3. Add CODEOWNERS entries so changes to the spec, the lock and workflows need
     a maintainer's review:

       /pinfold.yml          @your-org/maintainers
       /pinfold.lock         @your-org/maintainers
       /.github/workflows/   @your-org/maintainers

  4. For each environment a job names (for example pypi), open Settings >
     Environments and add protection rules: required reviewers, and
     "Deployment branches and tags" limited to your release tags (e.g. v*).
     Enter the same environment name in the service's trusted publisher
     (PyPI) or OIDC trust policy, so it trusts only that environment.
  5. Protect your release tags with a tag ruleset (Settings > Rules >
     Rulesets, target tags such as v*) using "Restrict creations",
     "Restrict updates" and "Restrict deletions", so only maintainers can
     push the tags that start a release.

Update bots: leave pinfold's generated files to "pinfold lock --update" (milestone M4).
  Renovate (renovate.json):
       "ignorePaths": [".github/workflows/pinfold-*.yml"]
  Dependabot (.github/dependabot.yml, github-actions entry):
       exclude-paths:
         - ".github/workflows/pinfold-*.yml"
     (Dependabot security updates may ignore exclude-paths; from M4,
      "pinfold check" reports any edit they make to a generated file.)
`

// Run writes the starter spec to path and prints the setup instructions.
// It refuses to overwrite an existing file.
func Run(path string, stdout io.Writer) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302 G304 -- a spec is a normal, user-named repository file
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists; not overwriting it", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(Starter); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, Instructions, path, path)
	return err
}
