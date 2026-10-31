// Effect: release. Creates the release, then uploads matching files from the workspace.
const fs = require('fs');
const path = require('path');
const tag = val('TAG');
if (!S.validTag(tag)) throw new Error('pinfold: refusing to create a release for an invalid tag name');
let release;
try {
  release = await github.rest.repos.createRelease({
    owner, repo, tag_name: tag,
    generate_release_notes: cfg('GENERATE_NOTES', 'false') === 'true',
    draft: cfg('DRAFT', 'false') === 'true',
    prerelease: cfg('PRERELEASE', 'false') === 'true',
  });
} catch (e) {
  if (e.status === 404 || e.status === 403) {
    core.error('pinfold: GitHub refused to create the release. If the tagged commit changes files under .github/workflows/ relative to the default branch, the GITHUB_TOKEN cannot create it (GitHub REST docs, "Create a release").');
  }
  throw e;
}
const patterns = json('FILES', []);
if (patterns.length === 0) return;
const workspace = fs.realpathSync(process.env.GITHUB_WORKSPACE || process.cwd());
const globber = await glob.create(patterns.join('\n'), { matchDirectories: false });
for (const file of await globber.glob()) {
  const real = fs.realpathSync(file);
  if (!real.startsWith(workspace + path.sep)) throw new Error('pinfold: refusing to upload a file outside the workspace');
  const data = fs.readFileSync(real);
  await github.rest.repos.uploadReleaseAsset({
    owner, repo, release_id: release.data.id, name: path.basename(real), data,
    headers: { 'content-type': 'application/octet-stream', 'content-length': data.length },
  });
}
