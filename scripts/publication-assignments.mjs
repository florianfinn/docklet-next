// Source: docklet-hub/scripts/check-publication.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Linear literal scanner with source-expression and YAML context.
// Scan literal credentials without treating source expressions as stored values.
const SENSITIVE_NAME = /password|passwd|secret|token|api[_-]?key|private[_-]?key|passphrase|access[_-]?key|pwd|credentials/i;
const VALUE_END = /[\s"'\x60,;)&]/;
const SOURCE_PATH = /\.(?:[cm]?[jt]sx?|go|rs|java|cs|py|rb|php|vue|svelte)$/i;
function placeholder(value) {
  return (
    !value ||
    ["changeme", "example", "test-only"].includes(value.toLowerCase()) ||
    /^\*+$/.test(value) ||
    /^<[^<>]*>$/.test(value) ||
    /^\$\{[a-z0-9_]+(?::[-+?=][^{}]*)?\}$/i.test(value) ||
    /^\$\{\{[^{}]+\}\}$/.test(value)
  );
}
export function scanAssignments(text, report, path = "text") {
  const source = SOURCE_PATH.test(path);
  const lines = text.split(/\r\n|\r|\n/);
  for (let row = 0; row < lines.length; row++) {
    const line = lines[row];
    const codeLine = source || /^\s*(?:export\s+)?(?:const|let|var|if|return|func|function|type|interface)\b/.test(line);
    const words = /[a-z0-9_.-]+/gi;
    let match;
    while ((match = words.exec(line))) {
      if (!SENSITIVE_NAME.test(match[0])) continue;
      let at = words.lastIndex;
      if (line[at] === '"' || line[at] === "'") at++;
      const end = at;
      while (line[at] === " " || line[at] === "\t") at++;
      const flag = match[0].startsWith("--");
      const separator = line[at];
      let declaration = false;
      if (separator === "=" || separator === ":") {
        at++;
        if (separator === ":" && line[at] === "=") {
          declaration = true;
          at++;
        }
        if (line[at] === "=" || line[at] === ">") continue;
        while (line[at] === " " || line[at] === "\t") at++;
      } else if (!(flag && at > end)) continue;
      // Indented YAML continuation and block scalars belong to this key.
      if (!codeLine && !flag && separator === ":" && /^(?:[>|][+-]?)?\s*(?:#.*)?$/.test(line.slice(at))) {
        const indent = line.length - line.trimStart().length;
        while (row + 1 < lines.length) {
          const next = lines[row + 1];
          if (next.trim() && next.length - next.trimStart().length <= indent) break;
          row++;
          let value = next.trim();
          if (line.slice(at).trim() === "" && /^(["']).*\1$/.test(value)) value = value.slice(1, -1);
          if (value && !value.startsWith("#") && !placeholder(value))
            report("literal-secret", line.slice(match.index) + "\n" + next);
        }
        words.lastIndex = line.length;
        continue;
      }
      const start = at;
      const quote = line[at];
      let value;
      if (quote === '"' || quote === "'" || ((codeLine || declaration) && quote === "`")) {
        const first = ++at;
        while (at < line.length && line[at] !== quote) {
          if (line[at] === "\\" && at + 1 < line.length) at++;
          at++;
        }
        value = line.slice(first, at);
        if (line[at] === quote) at++;
      } else {
        if (line[at] === "$" && line[at + 1] === "{") {
          let depth = 1;
          at += 2;
          while (at < line.length && depth) {
            if (line[at] === "{") depth++;
            if (line[at] === "}") depth--;
            at++;
          }
        } else if (line[at] === "<") {
          while (at < line.length && line[at] !== ">") at++;
          if (line[at] === ">") at++;
        }
        while (at < line.length && !VALUE_END.test(line[at]) && line[at] !== "}") at++;
        value = line.slice(start, at);
      }
      words.lastIndex = Math.max(words.lastIndex, at);
      const quoted = quote === '"' || quote === "'" || quote === "`";
      // Bare source values include identifiers, calls, member access and type annotations.
      const ambiguousCode = path === "text" && (
        /^(?:string|number|boolean|unknown|never|any|void)(?:\[\])?$/.test(value) ||
        /[([]/.test(line[at] ?? "") || /[([]/.test(value));
      const expression = !flag && !quoted && (codeLine || declaration || ambiguousCode ||
        /^(?:true|false|null|undefined)$/.test(value));
      if (!expression && !placeholder(value)) report("literal-secret", line.slice(match.index, at));
    }
  }
}
