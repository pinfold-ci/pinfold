// Effect: commit_status.
const sha = val('SHA');
if (!/^[0-9a-f]{40}$/.test(sha)) throw new Error('pinfold: commit_status needs a full 40-character commit SHA');
const state = cfg('STATE', '');
if (!['error', 'failure', 'pending', 'success'].includes(state)) throw new Error('pinfold: invalid commit status state');
const description = S.capBytes(S.stripControl(cfg('DESCRIPTION', '')).replace(/\n/g, ' '), 140);
await github.rest.repos.createCommitStatus({ owner, repo, sha, state, context: cfg('CONTEXT', 'pinfold'), description });
