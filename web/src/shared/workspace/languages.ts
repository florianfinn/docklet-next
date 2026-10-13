import { yaml } from "@codemirror/lang-yaml";
import { StreamLanguage, type StringStream } from "@codemirror/language";
import type { Extension } from "@codemirror/state";

/** Languages the shared editor highlights; everything else opens as plain text. */
export type EditorLanguage = "yaml" | "json" | "env" | "ini" | "shell" | "text";

export function languageFor(fileName: string): EditorLanguage {
  const name = fileName.toLowerCase();
  if (/\.ya?ml$/.test(name)) return "yaml";
  if (/\.json$/.test(name)) return "json";
  if (name === ".env" || name.startsWith(".env.") || name.endsWith(".env")) return "env";
  if (/\.(ini|conf|cfg|toml|properties|service)$/.test(name)) return "ini";
  if (/\.(sh|bash)$/.test(name)) return "shell";
  return "text";
}

const VALUE_WORD = /^(true|false|yes|no|on|off)\b/i;

/** KEY=value files (.env, ini, conf, toml): comments, sections, keys and typed values. */
const keyValue = StreamLanguage.define<{ value: boolean }>({
  name: "key-value",
  startState: () => ({ value: false }),
  token(stream: StringStream, state) {
    if (stream.sol()) state.value = false;
    if (stream.eatSpace()) return null;
    if (!state.value) {
      if (stream.match(/^[#;].*/)) return "comment";
      if (stream.match(/^\[[^\]]*\]?/)) return "heading";
      if (stream.match(/^export\b/)) return "keyword";
      if (stream.match(/^[^=:\s#;]+/)) return "propertyName";
      if (stream.match(/^[=:]/)) {
        state.value = true;
        return "operator";
      }
      stream.next();
      return null;
    }
    if (stream.match(/^#.*/)) return "comment";
    if (stream.match(/^"(?:[^"\\]|\\.)*"?/) || stream.match(/^'[^']*'?/)) return "string";
    if (stream.match(/^\$\{?[A-Za-z_]\w*\}?/)) return "variableName";
    if (stream.match(/^-?\d+(?:\.\d+)?(?![\w.])/)) return "number";
    if (stream.match(VALUE_WORD)) return "bool";
    if (!stream.match(/^[^\s#"'$]+/)) stream.next();
    return "string";
  },
  languageData: { commentTokens: { line: "#" } },
});

const SHELL_KEYWORDS = /^(if|then|else|elif|fi|for|while|until|do|done|case|esac|in|function|return|exit|export|local|set|readonly)\b/;

const shell = StreamLanguage.define<null>({
  name: "shell",
  startState: () => null,
  token(stream) {
    if (stream.eatSpace()) return null;
    if (stream.match(/^#.*/)) return "comment";
    if (stream.match(/^"(?:[^"\\]|\\.)*"?/) || stream.match(/^'[^']*'?/)) return "string";
    if (stream.match(/^\$\{?[\w@#?$!*-]+\}?/)) return "variableName";
    if (stream.match(SHELL_KEYWORDS)) return "keyword";
    if (stream.match(/^-?\d+(?![\w.])/)) return "number";
    if (stream.match(/^[|&;<>()]+/)) return "operator";
    if (!stream.match(/^[^\s"'$#|&;<>()]+/)) stream.next();
    return null;
  },
  languageData: { commentTokens: { line: "#" } },
});

export function languageExtension(language: EditorLanguage): Extension {
  switch (language) {
    // JSON is a YAML 1.2 subset, so the YAML grammar highlights it without another dependency.
    case "yaml":
    case "json":
      return yaml();
    case "env":
    case "ini":
      return keyValue;
    case "shell":
      return shell;
    default:
      return [];
  }
}
