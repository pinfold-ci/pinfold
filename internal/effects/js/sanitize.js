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
    return s.replace(/\r\n?/g, '\n')
      .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F\u00AD\u034F\u061C\u115F\u1160\u17B4\u17B5\u180B-\u180F\u200B-\u200F\u2028-\u202E\u2060-\u206F\u3164\uFE00-\uFE0F\uFEFF\uFFA0\uFFF9-\uFFFB]/g, '')
      // Tag characters (invisible "ASCII smuggling") and supplementary variation selectors.
      .replace(/[\u{E0000}-\u{E007F}\u{E0100}-\u{E01EF}]/gu, '');
  }

  // Escape & and < so untrusted text can't spell @ or # as an HTML entity
  // (&#64;, &commat;, &num;) or carry raw HTML (links in href=, hidden
  // elements). Markdown still renders; HTML in the text shows as text.
  function escapeHtml(s) {
    return s.replace(/&(?!amp;|lt;)/g, '&amp;').replace(/</g, '&lt;');
  }

  // Insert a zero-width space after @ and # so mentions don't notify and refs
  // don't link, unless allowlisted. Input is stripped of zero-width characters
  // first (stripControl), so the only ones left are ours.
  function neutralise(s, allowMentions, allowRefs) {
    const mentions = new Set((allowMentions || []).map((m) => String(m).toLowerCase()));
    const refs = new Set((allowRefs || []).map(String));
    // owner/repo#12 is a cross-repository reference, so refs are neutralised
    // even after a word character. Entities are already escaped (escapeHtml).
    s = s.replace(/#(\d+)/g, (m, num) => (refs.has(num) ? m : '#\u200b' + num));
    s = s.replace(/\bGH-(\d+)/gi, (m, num) => (refs.has(num) ? m : m.slice(0, 3) + '\u200b' + num));
    // Every @name, whatever precedes it: GitHub notifies for _@user_ (the _
    // is a word character), so no lookbehind (security audit NV1). This also
    // breaks e-mail autolinks, which costs nothing in a bot comment.
    s = s.replace(/@([A-Za-z0-9](?:[A-Za-z0-9-]{0,38})(?:\/[A-Za-z0-9._-]+)?)/g, (m, name) =>
      mentions.has(name.toLowerCase()) ? m : '@\u200b' + name);
    return s;
  }

  // Keep at most max links; later ones become plain text. Counts scheme
  // links, www. autolinks and protocol-relative Markdown link targets.
  const LINK_START = String.raw`(?:https?:\/\/|(?<![\w.\-])www\.|(?<=\]\()\/\/)`;
  const LINK_RE = new RegExp(LINK_START + String.raw`(?:(?!` + LINK_START + String.raw`)[^\s<>()\[\]` + '`' + String.raw`"'])*`, 'gi');
  function capLinks(s, max) {
    let n = 0;
    return s.replace(LINK_RE, (url) => (++n <= max ? url : '[link removed]'));
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
    // Drop a mention or reference cut at the end: an allowlisted @torvaldsx1 or
    // #1234 cut short would be a live mention of someone else, or a link to
    // another issue (external audit I6). A neutralised one goes too.
    out = out.replace(/(?:[@#]|\bGH-)\u200b?[A-Za-z0-9._\/-]*$/i, '');
    // Nor on part of an entity escapeHtml wrote (&amp; or &lt; cut short).
    out = out.replace(/&(?:a|am|amp|l|lt)?$/, '');
    return out + marker;
  }

  function sanitizeBody(text, opts = {}) {
    let s = String(text ?? '');
    s = stripControl(s);
    s = stripHtmlComments(s);
    s = escapeHtml(s);
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

  // A tag name git itself accepts (git check-ref-format), within TAG_RE.
  function validTag(tag) {
    if (typeof tag !== 'string' || !TAG_RE.test(tag) || tag.includes('..') || tag.startsWith('-') || tag.endsWith('.')) return false;
    return tag.split('/').every((c) => c !== '' && !c.startsWith('.') && !c.endsWith('.lock'));
  }

  // Render {{ name }} placeholders with control-stripped values; unknown names render empty.
  function renderTemplate(tpl, vars) {
    return String(tpl).replace(/\{\{\s*([a-z_.]+)\s*\}\}/g, (_, name) => stripControl(String(vars[name] ?? '')));
  }

  return { sanitizeBody, labelsFromComment, filterLabels, validTag, renderTemplate, stripHtmlComments, stripControl, escapeHtml, capBytes, capLinks, neutralise };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = PINFOLD_SANITIZER;
