// Pinfold effect sanitiser. Inlined, unchanged, into every generated effect
// script; also loaded by sanitize.test.js. Pure functions, no I/O.
const PINFOLD_SANITIZER = (() => {
  const DEFAULT_MAX_BYTES = 16384;
  const DEFAULT_MAX_LINKS = 10;
  const TAG_RE = /^[A-Za-z0-9._\/-]{1,128}$/;
  const LABEL_RE = /^[^\s,][^,\n\r]{0,49}$/;

  // Remove HTML comments, including an unterminated one at the end.
  function stripHtmlComments(s) {
    const out = s.replace(/<!--[\s\S]*?-->/g, '');
    const open = out.indexOf('<!--');
    return open === -1 ? out : out.slice(0, open);
  }

  // Remove control and invisible formatting characters except tab and newline; normalise CR.
  function stripControl(s) {
    return s.replace(/\r\n?/g, '\n').replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F\u200B-\u200F\u2028-\u202E\u2060-\u2069\uFEFF]/g, '');
  }

  // Wrap @mentions and #refs in backticks unless allowlisted, so they don't
  // notify people or link issues. Allowlists hold names without the prefix.
  // Insert a zero-width space after @ and # so mentions don't notify and refs
  // don't link, unless allowlisted. Input is stripped of zero-width characters
  // first (stripControl), so the only ones left are ours.
  function neutralise(s, allowMentions, allowRefs) {
    const mentions = new Set((allowMentions || []).map((m) => String(m).toLowerCase()));
    const refs = new Set((allowRefs || []).map(String));
    // owner/repo#12 is a cross-repository reference, so refs are neutralised
    // even after a word character; &#12; is an HTML entity, not a ref.
    s = s.replace(/(?<!&)#(\d{1,10})(?!\d)/g, (m, num) => (refs.has(num) ? m : '#\u200b' + num));
    s = s.replace(/(?<!\w)@([A-Za-z0-9](?:[A-Za-z0-9-]{0,38})(?:\/[A-Za-z0-9._-]+)?)/g, (m, name) =>
      mentions.has(name.toLowerCase()) ? m : '@\u200b' + name);
    return s;
  }

  // Keep at most max links; later ones become plain text.
  function capLinks(s, max) {
    let n = 0;
    return s.replace(/https?:\/\/(?:(?!https?:\/\/)[^\s<>()\[\]`"'])*/gi, (url) => (++n <= max ? url : '[link removed]'));
  }

  // Truncate to max UTF-8 bytes without splitting a character.
  function capBytes(s, max) {
    const enc = new TextEncoder();
    if (enc.encode(s).length <= max) return s;
    const marker = '\n\n[truncated]';
    const budget = max - enc.encode(marker).length;
    let bytes = 0;
    let out = '';
    for (const ch of s) {
      const len = enc.encode(ch).length;
      if (bytes + len > budget) break;
      bytes += len;
      out += ch;
    }
    // Don't end on a dangling neutralised @ or # (a cut right after it).
    out = out.replace(/[@#]\u200b?$/, '');
    return out + marker;
  }

  function sanitizeBody(text, opts = {}) {
    let s = String(text ?? '');
    s = stripControl(s);
    s = stripHtmlComments(s);
    s = capLinks(s, opts.maxLinks ?? DEFAULT_MAX_LINKS);
    s = neutralise(s, opts.allowMentions, opts.allowRefs);
    s = capBytes(s, opts.maxBytes ?? DEFAULT_MAX_BYTES);
    return s;
  }

  // Parse labels from a command comment: first line, after the command prefix.
  function labelsFromComment(body, prefix) {
    const first = String(body ?? '').split(/\r?\n/, 1)[0];
    const rest = first.startsWith(prefix) ? first.slice(prefix.length) : '';
    return rest.split(/[\s,]+/).filter(Boolean);
  }

  // Keep requested labels that are exactly in the allowlist, deduplicated, at most max.
  function filterLabels(requested, allowed, max) {
    const ok = new Set(allowed);
    const out = [];
    const rejected = [];
    for (const l of requested) {
      if (!ok.has(l) || !LABEL_RE.test(l)) { rejected.push(l); continue; }
      if (!out.includes(l)) out.push(l);
    }
    return { labels: out.slice(0, max), rejected, dropped: out.slice(max) };
  }

  function validTag(tag) {
    return typeof tag === 'string' && TAG_RE.test(tag) && !tag.includes('..') && !tag.startsWith('/') && !tag.endsWith('/');
  }

  // Render {{ name }} placeholders with control-stripped values; unknown names render empty.
  function renderTemplate(tpl, vars) {
    return String(tpl).replace(/\{\{\s*([a-z_.]+)\s*\}\}/g, (_, name) => stripControl(String(vars[name] ?? '')));
  }

  return { sanitizeBody, labelsFromComment, filterLabels, validTag, renderTemplate, stripHtmlComments, stripControl, capBytes, capLinks, neutralise };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = PINFOLD_SANITIZER;
