// Maintainer gate (design section 4). Sets output ok=true only when the user who
// triggered this run has an allowed role on the repository. Fails closed.
const actor = val('ACTOR');
const triggering = val('TRIGGERING_ACTOR');
const roles = json('ROLES', ['admin', 'maintain', 'write']);
// When the event carries content (a comment, review, issue, pull request or
// discussion), the job acts on it, so it must be new and written by the
// triggering user: otherwise deleting or editing someone else's command, or
// labelling someone else's issue, would run it as the maintainer (#64).
// Checked in this order because a comment event also carries its issue.
const payload = context.payload || {};
const content = [
  ['comment', 'created'], ['review', 'submitted'], ['discussion', 'created'], ['issue', 'opened'], ['pull_request', 'opened'],
].find(([k]) => payload[k]);
const item = content && payload[content[0]];
const author = item && item.user && item.user.login;
let ok = false;
if (!actor || actor !== triggering) {
  core.notice('pinfold: maintainer gate: this run was started or re-run by a different user; skipping');
} else if (content && (payload.action !== content[1] || typeof author !== 'string' || author.toLowerCase() !== actor.toLowerCase())) {
  core.notice('pinfold: maintainer gate: only new content written by the triggering user counts; skipping');
} else if (/\[bot\]$/i.test(actor) || !payload.sender || payload.sender.type !== 'User') {
  core.notice('pinfold: maintainer gate: bots and apps are not maintainers; skipping');
} else {
  try {
    const r = await github.rest.repos.getCollaboratorPermissionLevel({ owner, repo, username: triggering });
    ok = r.status === 200 && roles.includes(r.data.role_name);
    if (!ok) core.notice('pinfold: maintainer gate: the triggering user is not a maintainer; skipping');
  } catch (e) {
    core.notice('pinfold: maintainer gate: permission lookup failed (status ' + Number(e.status) + '); skipping');
  }
}
core.setOutput('ok', ok ? 'true' : 'false');
