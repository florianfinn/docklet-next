// Source: docklet-hub/scripts/check-publication.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Context-based literal decision table; see publication-policy.md.
const SENSITIVE_NAME = /password|passwd|secret|token|api[_-]?key|private[_-]?key|passphrase|access[_-]?key|(?:^|[_-])pwd$|credentials$/i;
const SOURCE_PATH = /\.(?:[cm]?[jt]sx?|go|rs|java|cs|py|rb|php|vue|svelte)$/i;
const VALUE_END = /[\s"'\x60,;)&}\]]/;
function placeholder(value) {
  if (!value || /^(?:changeme|example|test-only)$/i.test(value) ||
      /^\*+$/.test(value) || /^<[^<>]*>$/.test(value) || /^\$\{\{[^{}]+\}\}$/.test(value)) return true;
  const env = value.match(/^\$\{[a-z0-9_]+(?:(:?[-+?=])([^{}]*))?\}$/i);
  return !!env && (!env[1] || placeholder(env[2]));
}
// 0 = source syntax, 1 = string content, 2 = comment. State spans raw strings.
function contexts(line, state) {
  const result = new Uint8Array(line.length);
  result.leading = new Set();
  result.lineBreaks = new Set();
  let leading = true;
  for (let i = 0; i < line.length; i++) {
    if (state.block) {
      result[i] = 2;
      if (line[i] === "*" && line[i + 1] === "/") { result[++i] = 2; state.block = false; }
    } else if (state.quote) {
      result[i] = 1;
      if (leading && !/\s/.test(line[i])) {
        result.leading.add(i);
        const prefix = line.slice(i).match(/^default[ \t]+/i);
        if (prefix) result.leading.add(i + prefix[0].length);
        leading = false;
      }
      if (line[i] === "\\" && state.quote !== "`") {
        if (line[i + 1] === "n" || line[i + 1] === "r") { result.lineBreaks.add(i); leading = true; }
        if (i + 1 < line.length) result[++i] = 1;
      }
      else if (line[i] === state.quote) state.quote = "";
    } else if (line[i] === "#" || line.slice(i, i + 2) === "//") {
      result.fill(2, i); break;
    } else if (line.slice(i, i + 2) === "/*") {
      result[i] = result[++i] = 2; state.block = true;
    } else if ('"\'`'.includes(line[i])) { state.quote = line[i]; leading = true; }
  }
  return result;
}
function readValue(line, start, lineBreaks) {
  let at = start;
  const prefix = line.slice(at).match(/^(?:br|rb|[bruf])(?=["'])/i);
  if (prefix) at += prefix[0].length;
  const quote = line[at];
  const quoted = quote && '"\'`'.includes(quote);
  if (quoted) {
    const first = ++at;
    while (at < line.length && line[at] !== quote) {
      if (line[at] === "\\" && at + 1 < line.length) at++;
      at++;
    }
    return { value: line.slice(first, at), end: Math.min(at + 1, line.length), quoted: true, closed: line[at] === quote };
  }
  if (line[at] === "$" && line[at + 1] === "(") {
    let depth = 1; at += 2;
    while (at < line.length && depth) {
      if (line[at] === "(") depth++;
      if (line[at] === ")") depth--;
      at++;
    }
  } else if (line[at] === "$" && line[at + 1] === "{") {
    let depth = 1; at += 2;
    while (at < line.length && depth) {
      if (line[at] === "{") depth++;
      if (line[at] === "}") depth--;
      at++;
    }
  } else if (line[at] === "<") {
    while (at < line.length && line[at] !== ">") at++;
    if (line[at] === ">") at++;
  }
  while (at < line.length && !VALUE_END.test(line[at]) && !lineBreaks?.has(at)) at++;
  return { value: line.slice(start, at), end: at, quoted: false };
}
export function scanAssignments(text, report, path = "text") {
  const source = SOURCE_PATH.test(path);
  const lines = text.split(/\r\n|\r|\n/);
  const state = { quote: "", block: false };
  const offsets = [];
  let cursor = 0;
  for (const line of lines) {
    offsets.push(cursor);
    cursor += line.length + (text[cursor + line.length] === "\r" && text[cursor + line.length + 1] === "\n" ? 2 : 1);
  }
  if (source) scanNamedValues(text, report);
  for (let row = 0; row < lines.length; row++) {
    const line = lines[row];
    const offset = offsets[row];
    let consumed = row;
    const context = source ? contexts(line, state) : null;
    const words = /[a-z0-9_.-]+/gi;
    let match;
    while ((match = words.exec(line))) {
      // Escaped line endings delimit configuration keys and unquoted values.
      if (context?.lineBreaks.has(match.index - 1)) { match[0] = match[0].slice(1); match.index++; }
      const name = match[0];
      if (!SENSITIVE_NAME.test(name) || line.slice(match.index - 2, match.index) === "${") continue;
      let at = words.lastIndex;
      const quotedKey = (line[at] === '"' || line[at] === "'") && line[match.index - 1] === line[at];
      if (quotedKey) at++;
      const end = at;
      while (/[ \t]/.test(line[at] ?? "")) at++;
      const flag = name.startsWith("--");
      const embedded = source && context[match.index] !== 0 && !quotedKey;
      const code = source && !embedded;
      let separator = line[at];
      // A typed Go declaration puts its type between the name and equals.
      if (code && !flag && separator !== ":" && separator !== "=") {
        const type = line.slice(at).match(/^[\w.\[\]*]+[ \t]+=(?!=|>)/);
        if (type) { at += type[0].length - 1; separator = "="; }
      }
      if (separator === "=" || separator === ":") {
        at++;
        if (separator === ":" && line[at] === "=") at++;
        if (line[at] === "=" || line[at] === ">" || line[at] === ":") continue;
        while (/[ \t]/.test(line[at] ?? "")) at++;
        if (code && separator === ":") {
          const type = line.slice(at).match(/^[\w.\[\]<>|?&' \t]{1,256}=(?!=|>)/);
          if (type) { at += type[0].length; while (/[ \t]/.test(line[at] ?? "")) at++; }
        }
      } else if (!(flag && at > end)) continue;
      if (code && !line.slice(at).trim()) {
        let nextRow = row + 1;
        while (nextRow < lines.length && !lines[nextRow].trim()) nextRow++;
        if (nextRow < lines.length) {
          const next = lines[nextRow];
          const scalar = readValue(next, next.length - next.trimStart().length);
          if (scalar.quoted && !placeholder(scalar.value)) report("literal-secret", next, offsets[nextRow]);
        }
      }
      if (!code && !flag && separator === ":" && /^(?:[>|][+-]?)?\s*(?:#.*)?$/.test(line.slice(at))) {
        const indent = line.length - line.trimStart().length;
        const block = /^[>|]/.test(line.slice(at));
        let nextOffset = offset + line.length + (text[offset + line.length] === "\r" && text[offset + line.length + 1] === "\n" ? 2 : 1);
        for (let nextRow = row + 1; nextRow < lines.length; nextRow++) {
          const next = lines[nextRow];
          const value = next.trim();
          if (value && next.length - next.trimStart().length <= indent) break;
          // A structural mapping/list is not a scalar credential value.
          if (!block && /^(?:-\s|[^:]+:\s|[^:]+:$)/.test(value)) break;
          if (value && !value.startsWith("#")) {
            const scalar = /^(["']).*\1$/.test(value) ? value.slice(1, -1) : value;
            if (!placeholder(scalar)) report("literal-secret", next, nextOffset);
          }
          consumed = nextRow;
          nextOffset = offsets[nextRow + 1];
        }
        continue;
      }
      if (!code && separator === ":" && (line[at] === "{" || line[at] === "[")) continue;
      // An enclosing string's closing quote is not a new value opener.
      if (embedded && context[at] === 1 && !context[at + 1] && '"\'`'.includes(line[at] ?? "\0")) continue;
      // Leading keys and "Default key" labels are configuration; other prose colons are punctuation.
      if (source && context[match.index] === 1 && !quotedKey && separator === ":" &&
          (line[at] === "%" || !context.leading.has(match.index))) continue;
      const { value, end: valueEnd, quoted, closed } = readValue(line, at, context?.lineBreaks);
      if (code && line[at] === "`" && !closed) {
        let nextOffset = offset + line.length + (text[offset + line.length] === "\r" && text[offset + line.length + 1] === "\n" ? 2 : 1);
        for (let nextRow = row + 1; nextRow < lines.length; nextRow++) {
          const next = lines[nextRow];
          const end = next.indexOf("`");
          const scalar = (end < 0 ? next : next.slice(0, end)).trim();
          if (!placeholder(scalar)) report("literal-secret", next, nextOffset);
          if (end >= 0) break;
          nextOffset += next.length + (text[nextOffset + next.length] === "\r" && text[nextOffset + next.length + 1] === "\n" ? 2 : 1);
        }
      }
      words.lastIndex = Math.max(words.lastIndex, valueEnd);
      const expression = !flag && !quoted && (code || /^(?:true|false|null|undefined)$/.test(value) ||
        (path === "text" && (/^(?:string|number|boolean|unknown|never|any|void)(?:\[\])?$/.test(value) || /[([]/.test(line[valueEnd] ?? "") || /[([]/.test(value))));
      const credentialMode = /credentials$/i.test(name) && /^(?:include|same-origin|omit)$/.test(value);
      const credentialPath = /(?:credentials|pwd|[_-](?:file|path))$/i.test(name) && /^(?:\/|\.\.?\/|[A-Z]:[\\/])/.test(value);
      const shellReference = !source && (/^\$[a-z_][a-z0-9_]*$/i.test(value) || /^\$\([\s\S]*\)$/.test(value) ||
        (line[at] === "`" && /(?:\.(?:sh|bash|zsh)|(?:^|\/)Dockerfile[^/]*)$/.test(path)));
      const workflowMode = !source && ((name === "id-token" && /^(?:write|read|none)$/.test(value)) || (name === "secrets" && value === "inherit"));
      if (!expression && !credentialMode && !credentialPath && !shellReference && !workflowMode && !placeholder(value))
        report("literal-secret", line.slice(match.index, valueEnd), offset + match.index);
    }
    row = consumed;
  }
}

// Recognize literal setter arguments and adjacent named-value fields, without data flow.
function scanNamedValues(text, report) {
  const patterns = [
    /\.(?:Set|setItem)\(\s*["']([a-z0-9_-]+)["']\s*,\s*/gi,
    /["']?(?:key|name)["']?\s*:\s*["']([a-z0-9_-]+)["'](?:\s*,)?\s*["']?value["']?\s*:\s*/gi,
  ];
  for (const pattern of patterns) {
    let match;
    while ((match = pattern.exec(text))) {
      if (!SENSITIVE_NAME.test(match[1])) continue;
      const scalar = readValue(text, pattern.lastIndex);
      if (scalar.quoted && !placeholder(scalar.value)) report("literal-secret", text.slice(pattern.lastIndex, scalar.end), pattern.lastIndex);
      pattern.lastIndex = Math.max(pattern.lastIndex, scalar.end);
    }
  }
}
