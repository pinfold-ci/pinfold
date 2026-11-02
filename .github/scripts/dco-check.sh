#!/usr/bin/env bash
# Checks that every non-merge commit in BASE..HEAD has a Developer Certificate
# of Origin sign-off by its author: a "Signed-off-by: Name <email>" line whose
# email matches the commit's author email (case-insensitive).
# Usage: dco-check.sh BASE_SHA HEAD_SHA
set -euo pipefail

base=$1
head=$2
missing=0
for sha in $(git rev-list --no-merges "$base..$head"); do
  author=$(git show -s --format='%ae' "$sha" | tr '[:upper:]' '[:lower:]')
  if git show -s --format='%B' "$sha" |
    grep -iE '^Signed-off-by: .+ <[^>]+>[[:space:]]*$' |
    sed -E 's/.*<([^>]+)>.*/\1/' | tr '[:upper:]' '[:lower:]' |
    grep -qxF "$author"; then
    continue
  fi
  echo "::error::commit $sha has no Signed-off-by line for its author <$author>"
  missing=1
done
if [ "$missing" -ne 0 ]; then
  cat <<'MSG'
Every commit needs a sign-off by its author (Developer Certificate of Origin).
Add it to the commits in this pull request and force-push:

  git rebase --signoff origin/main
  git push --force-with-lease

See CONTRIBUTING.md, "Developer Certificate of Origin".
MSG
  exit 1
fi
echo "All commits are signed off."
