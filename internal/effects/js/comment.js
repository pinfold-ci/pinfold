// Effect: comment (optionally sticky: one comment, updated in place).
const issue_number = issueNumber();
const names = json('VARS', []);
const values = {};
names.forEach((n, i) => { values[n] = val('V' + i); });
// Only the triggering user may be mentioned (design section 5); every other @ is neutralised.
const allowMentions = names.includes('actor') ? [values.actor] : [];
let body = S.sanitizeBody(S.renderTemplate(cfg('BODY', ''), values), { allowMentions, maxBytes: 16000 });
const sticky = cfg('STICKY', '');
if (sticky) {
  const marker = '<!-- pinfold:sticky:' + sticky + ' -->';
  body = body + '\n' + marker;
  const comments = await github.paginate(github.rest.issues.listComments, { owner, repo, issue_number, per_page: 100 });
  // Match on the marker AND our bot author: anyone can paste the marker.
  const mine = comments.find((c) => c.user && c.user.login === 'github-actions[bot]' && c.user.type === 'Bot' && (c.body || '').includes(marker));
  if (mine) {
    try {
      await github.rest.issues.updateComment({ owner, repo, comment_id: mine.id, body });
      return;
    } catch (e) {
      if (e.status !== 404 && e.status !== 403) throw e;
    }
  }
}
await github.rest.issues.createComment({ owner, repo, issue_number, body });
