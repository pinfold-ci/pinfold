// Maintainer gate (design section 4). Sets output ok=true only when the user who
// triggered this run has an allowed role on the repository. Fails closed.
const actor = val('ACTOR');
const triggering = val('TRIGGERING_ACTOR');
const roles = json('ROLES', ['admin', 'maintain', 'write']);
const sender = context.payload && context.payload.sender;
let ok = false;
if (!actor || actor !== triggering) {
  core.notice('pinfold: maintainer gate: this run was started or re-run by a different user; skipping');
} else if (/\[bot\]$/i.test(actor) || !sender || sender.type !== 'User') {
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
