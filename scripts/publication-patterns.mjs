// Source: docklet-hub/scripts/check-publication.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Reserved networks only; IPv6, POSIX paths, corrupt text and literal credentials; no prose spelling guard.
import { isIP } from "node:net";
import { fixtureAllowed } from "./publication-fixtures.mjs";

const AI_TOOLS = new Set([
  "claude",
  "copilot",
  "codex",
  "chatgpt",
  "openai",
  "gemini",
  "cursor",
  "devin",
  "opus",
  "sonnet",
  "haiku",
  "fable",
  "grok",
  "llama",
  "deepseek",
]);
export function aiIdentity(name, email = "") {
  const words = name
    .trim()
    .toLowerCase()
    .replace(/^github\s+/, "")
    .split(/[\s_()-]+/)
    .filter(Boolean);
  const toolName =
    AI_TOOLS.has(words[0]) &&
    words
      .slice(1)
      .every(
        (w) =>
          /^(?:ai|agent|code|coding|cli|assist|assistant|gpt|bot|\[bot\]|opus|sonnet|haiku|pro|flash)$/.test(
            w,
          ) || /\d/.test(w),
      );
  const login = email
    .toLowerCase()
    .split("@")[0]
    .replace(/^\d+\+/, "");
  return (
    toolName ||
    ["anthropic.com", "openai.com", "cursor.com", "cursor.sh", "devin.ai", "cognition.ai"].some((domain) => {
      const host = email.slice(email.lastIndexOf("@") + 1).toLowerCase();
      return email.includes("@") && (host === domain || host.endsWith("." + domain));
    }) ||
    AI_TOOLS.has(login) ||
    (login.endsWith("[bot]") && [...AI_TOOLS, "jules"].some((t) => login.includes(t)))
  );
}
export function publicIdentity(name, email) {
  return (
    !aiIdentity(name, email) &&
    (/^[a-z0-9+._-]+@users\.noreply\.github\.com$/i.test(email) ||
      email.toLowerCase() === "noreply@github.com")
  );
}
const EXAMPLE_EMAIL =
  /@(?:users\.noreply\.github\.com|(?:[a-z0-9-]+\.)*example\.(?:com|net|org)|(?:[a-z0-9-]+\.)*(?:test|invalid))$/i;
// Token prefixes are anchored so failed candidates are not retried at every character.
const CREDENTIAL_START = String.raw`(?<![a-z0-9_-])`;
const CREDENTIAL_END = String.raw`(?![a-z0-9_-])`;
const CREDENTIALS = [
  /-----BEGIN (?:[A-Z0-9]+ )?PRIVATE KEY-----/.source,
  `${CREDENTIAL_START}(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,})${CREDENTIAL_END}`,
  `${CREDENTIAL_START}(?:AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{24,})${CREDENTIAL_END}`,
  `${CREDENTIAL_START}xox[abprs]-[A-Za-z0-9-]{20,}${CREDENTIAL_END}`,
  String.raw`\bBearer[ \t]+[A-Za-z0-9._-]{20,}`,
  `${CREDENTIAL_START}eyJ[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+${CREDENTIAL_END}`,
];
// Consume keys and values once, including CLI flags and unquoted YAML scalars.
const SENSITIVE_NAME = /password|passwd|secret|token|api[_-]?key|private[_-]?key/i;
const VALUE_END = /[\s"'\x60,;)&]/;
function placeholder(value) {
  const lower = value.toLowerCase();
  return (
    !value ||
    ["changeme", "example", "test-only"].includes(lower) ||
    /^\*+$/.test(value) ||
    /^<[^<>]*>$/.test(value) ||
    /^\$\{[a-z0-9_]+\}$/i.test(value) ||
    /^\$\{\{[^{}]+\}\}$/.test(value)
  );
}
function scanAssignments(text, report) {
  const words = /[a-z0-9_.-]+/gi;
  let match;
  while ((match = words.exec(text))) {
    if (!SENSITIVE_NAME.test(match[0])) continue;
    let at = words.lastIndex;
    if (text[at] === '"' || text[at] === "'") at++;
    const end = at;
    while (text[at] === " " || text[at] === "\t") at++;
    if (text[at] === "=" || text[at] === ":") {
      at++;
      while (text[at] === " " || text[at] === "\t") at++;
    } else if (!(match[0].startsWith("--") && at > end)) continue;
    const start = at;
    const quote = text[at];
    let value;
    if (quote === '"' || quote === "'") {
      at++;
      const first = at;
      while (at < text.length && text[at] !== quote && text[at] !== "\r" && text[at] !== "\n") at++;
      value = text.slice(first, at);
      if (text[at] === quote) at++;
    } else if (text[at] === "$" && text[at + 1] === "{") {
      let depth = 1;
      at += 2;
      while (at < text.length && depth && text[at] !== "\r" && text[at] !== "\n") {
        if (text[at] === "{") depth++;
        if (text[at] === "}") depth--;
        at++;
      }
      value = text.slice(start, at);
    } else if (text[at] === "<") {
      while (at < text.length && !/[>\r\n]/.test(text[at])) at++;
      if (text[at] === ">") at++;
      value = text.slice(start, at);
    } else {
      while (at < text.length && !VALUE_END.test(text[at]) && text[at] !== "}") at++;
      value = text.slice(start, at);
    }
    if (quote !== '"' && quote !== "'") {
      while (at < text.length && !VALUE_END.test(text[at]) && text[at] !== "}") at++;
      value = text.slice(start, at);
    }
    words.lastIndex = Math.max(words.lastIndex, at);
    if (!placeholder(value)) report("literal-secret", text.slice(match.index, at));
  }
}
// Home roots include Unix, macOS, Windows and shell home expansion.
const HOME_PATHS = [
  String.raw`/(?:home|Users|u)/[a-z0-9._-]+(?:/[^\s"'<>\x60,;()]*)?`,
  String.raw`/(?:root)/[^\s"'<>\x60,;()]*`,
  String.raw`~(?:[a-z0-9._-]+)?/[^\s"'<>\x60,;()]*`,
  String.raw`[A-Z]:[\\/](?:Users|Documents and Settings)[\\/][^\s"'\x60,;()]+`,
];
// Scan each complete hostname once; punctuation is not part of its suffix.
const HOSTNAME = /[a-z0-9-]+(?:\.[a-z0-9-]+)*/gi;
const PRIVATE_SUFFIX = /\.(?:lan|local|internal|home\.arpa)$/i;
const PATTERNS = {
  "credential-pattern": new RegExp(CREDENTIALS.join("|"), "g"),
  "workstation-path": new RegExp(HOME_PATHS.join("|"), "gi"),
  "corrupted-text": new RegExp(
    [
      String.raw`\uFFFD|\u00C3[\u0080-\u00BF]|\u00C2[\u0080-\u00BF]|\u00E2\u20AC.`,
      String.raw`[\u0001-\u0008\u000B\u000C\u000E-\u001F\u007F]`,
      String.raw`[\u200B-\u200F\u202A-\u202E\u2060-\u2069\uFEFF]`,
    ].join("|"),
    "gu",
  ),
  "session-link":
    /(?:claude\.ai\/code\/session[_/]|chatgpt\.com\/(?:s|share|codex\/tasks)\/|chat\.openai\.com\/share\/)/gi,
};
function safeAddress(value) {
  if (value === "0.0.0.0" || value === "::") return true;
  if (isIP(value) === 4) return /^(?:192\.0\.2\.|198\.51\.100\.|203\.0\.113\.|127\.)/.test(value);
  const canonical = new URL(`http://[${value}]/`).hostname.slice(1, -1);
  return canonical === "::1" || canonical.startsWith("2001:db8:");
}
export function inspectText(text, path = "text", denylist = []) {
  const findings = [];
  const report = (category, value) => {
    if (!fixtureAllowed(path, category, value)) findings.push({ path, category });
  };
  for (const [category, pattern] of Object.entries(PATTERNS)) {
    for (const match of text.matchAll(pattern)) report(category, match[0]);
  }
  scanAssignments(text, report);
  for (const match of text.matchAll(HOSTNAME)) {
    const dotfile = text[match.index - 1] === "." && match[0].toLowerCase() === "env." + "local";
    if (!dotfile && PRIVATE_SUFFIX.test(match[0])) report("private-hostname", match[0]);
  }
  for (const match of text.matchAll(/(?<![A-Z0-9._%+-])[A-Z0-9._%+-]+@(?:[A-Z0-9-]+\.)+[A-Z][A-Z0-9-]*/gi)) {
    if (!EXAMPLE_EMAIL.test(match[0]) && match[0].toLowerCase() !== "noreply@github.com")
      report("non-example-email", match[0]);
  }
  for (const match of text.matchAll(/(?<![\d.])(?:\d{1,3}\.){3}\d{1,3}(?!\d|\.\d)/g)) {
    if (isIP(match[0]) && !safeAddress(match[0])) report("network-address", match[0]);
  }
  // Maximal tokens avoid retrying overlapping IPv6 candidates. Valid addresses are short.
  for (const match of text.matchAll(/[a-z0-9:.]+/gi)) {
    if (match[0].length > 46 || !match[0].includes(":")) continue;
    const value = match[0].replace(/\.$/, "");
    const scopeName = text[match.index - 1] !== "[" && /^[a-f]+::[a-f]+$/i.test(value);
    if (!scopeName && isIP(value) === 6 && !safeAddress(value)) report("network-address", value);
  }
  // Split trailers at delimiters instead of overlapping whitespace captures.
  for (const line of text.split(/\r\n|\r|\n/)) {
    const trailer = line.trimStart();
    if (!trailer.toLowerCase().startsWith("co-authored-by:")) continue;
    const identity = trailer.slice(15).trim();
    const at = identity.indexOf("<");
    const end = identity.indexOf(">", at + 1);
    const name = at < 0 ? identity : identity.slice(0, at);
    const email = at >= 0 && end > at ? identity.slice(at + 1, end) : "";
    if (aiIdentity(name, email)) report("ai-co-author", line);
  }
  const folded = denylist.length ? text.toLowerCase() : "";
  for (const term of denylist)
    if (folded.includes(term.toLowerCase())) {
      findings.push({ path, category: "operator-denylist" });
      break;
    }
  return [...new Map(findings.map((f) => [f.category, f])).values()];
}
export function inspectPath(path, denylist = []) {
  const found = inspectText(path, "file-name", denylist);
  const label = found.length ? "file-name" : path;
  const basename = path.slice(path.lastIndexOf("/") + 1);
  const folded = basename.toLowerCase();
  if ((folded === ".env" || folded.startsWith(".env.")) && basename !== ".env.example")
    found.push({ path: label, category: "runtime-env-file" });
  if (
    path === ".publication-denylist.local.json" ||
    (path.startsWith(".claude/") && path !== ".claude/settings.json") ||
    /(?:^|\/)(?:\.private|review-reports)\//.test(path) ||
    /\.(?:pem|key|p12|dump|sqlite3?|log|zip|tgz)$/i.test(path)
  )
    found.push({ path: label, category: "private-artifact" });
  return found;
}
export function safePath(path, denylist = []) {
  return inspectText(path, "file-name", denylist).length || /[\r\n\x00-\x1f]/.test(path) ? "file-name" : path;
}
