// Pinfold effect helpers. Every input comes from the environment: T_* for
// untrusted values, PINFOLD_* for compiler-written configuration.
const S = PINFOLD_SANITIZER;
const { owner, repo } = context.repo;
function val(name) {
  const t = process.env['T_' + name];
  return t !== undefined ? t : (process.env['PINFOLD_' + name] || '');
}
function cfg(name, fallback) {
  const v = process.env['PINFOLD_' + name];
  return v === undefined || v === '' ? fallback : v;
}
function json(name, fallback) {
  const v = cfg(name, '');
  return v === '' ? fallback : JSON.parse(v);
}
function issueNumber() {
  const n = Number(val('TARGET'));
  if (!Number.isSafeInteger(n) || n <= 0) throw new Error('pinfold: the effect target is not an issue or pull request number');
  return n;
}
