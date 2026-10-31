// Effects: add_labels / remove_labels. Only labels in the allowlist, at most max.
const issue_number = issueNumber();
const requested = cfg('FROM_COMMENT', '') === 'true'
  ? S.labelsFromComment(val('COMMENT'), cfg('PREFIX', ''))
  : json('LABELS', []);
const allowed = json('ALLOWED', json('LABELS', []));
const max = Number(cfg('MAX', '10'));
const r = S.filterLabels(requested, allowed, max);
if (r.rejected.length > 0) core.warning('pinfold: ignored ' + r.rejected.length + ' label(s) that are not in the allowlist');
if (r.dropped.length > 0) core.warning('pinfold: ignored ' + r.dropped.length + ' label(s) over the limit of ' + max);
if (r.labels.length === 0) return;
if (cfg('MODE', 'add') === 'add') {
  await github.rest.issues.addLabels({ owner, repo, issue_number, labels: r.labels });
} else {
  for (const name of r.labels) {
    try {
      await github.rest.issues.removeLabel({ owner, repo, issue_number, name });
    } catch (e) {
      if (e.status !== 404) throw e;
    }
  }
}
