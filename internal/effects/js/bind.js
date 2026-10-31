// Artifact binding (design section 2): before a privileged job downloads an
// artifact by name, check that this run holds exactly one artifact with that
// name and that it is the one the producing job reported (id and digest).
const name = cfg('NAME', '');
const id = val('EXPECTED_ID');
const digest = val('EXPECTED_DIGEST').toLowerCase().replace(/^sha256:/, '');
if (!/^[a-z][a-z0-9-]{0,62}$/.test(name)) throw new Error('pinfold: invalid artifact name');
if (!/^[0-9]{1,20}$/.test(id)) throw new Error('pinfold: the producing job reported no valid artifact id');
if (!/^[0-9a-f]{64}$/.test(digest)) throw new Error('pinfold: the producing job reported no valid artifact digest');
const artifacts = await github.paginate(github.rest.actions.listWorkflowRunArtifacts, { owner, repo, run_id: context.runId, name, per_page: 100 });
const same = artifacts.filter((a) => a.name === name);
if (same.length !== 1) throw new Error('pinfold: expected exactly one artifact named ' + name + ' in this run, found ' + same.length);
const a = same[0];
const got = String(a.digest || '').toLowerCase().replace(/^sha256:/, '');
if (String(a.id) !== id || got !== digest) throw new Error('pinfold: artifact ' + name + ' is not the one its producing job uploaded');
