import { parse, TYPE, type MessageFormatElement } from "@formatjs/icu-messageformat-parser";

export type MessageTree = { [key: string]: string | string[] | MessageTree };

export function leaves(tree: MessageTree | string | string[], path = ""): [string, string][] {
  if (typeof tree === "string") return [[path, tree]];
  if (Array.isArray(tree)) return tree.map((value, i) => [`${path}[${i}]`, value]);
  return Object.entries(tree).flatMap(([key, value]) => leaves(value, path ? `${path}.${key}` : key));
}

/** Preserve branch context while allowing translators to reorder arguments. */
function shape(elements: MessageFormatElement[]): string[] {
  return [...new Set(elements.flatMap((element): string[] => {
    if (element.type === TYPE.literal) return [];
    if (element.type === TYPE.pound) return ["#"];
    const identity = `${element.value}:${TYPE[element.type]}`;
    if (element.type === TYPE.tag) return [`${identity}:${JSON.stringify(shape(element.children))}`];
    if (element.type === TYPE.select || element.type === TYPE.plural) {
      const kind = element.type === TYPE.plural ? `:${element.pluralType}:${element.offset}` : "";
      const options = Object.entries(element.options).sort(([a], [b]) => a.localeCompare(b))
        .map(([key, option]) => [key, shape(option.value)]);
      return [`${identity}${kind}:${JSON.stringify(options)}`];
    }
    return [identity];
  }))].sort();
}

export function icuShape(message: string): string[] {
  return shape(parse(message));
}

/** Literal apostrophes are doubled; ICU quoting is reserved for syntax characters. */
export function checkApostrophes(message: string): void {
  for (let i = 0; i < message.length; i++) {
    if (message[i] !== "'") continue;
    if (message[i + 1] === "'") { i++; continue; }
    if (!/[{}#<>]/.test(message[i + 1] ?? "")) throw new Error("undoubled apostrophe");
    let closed = false;
    while (++i < message.length) {
      if (message[i] !== "'") continue;
      if (message[i + 1] === "'") { i++; continue; }
      closed = true;
      break;
    }
    if (!closed) throw new Error("unclosed ICU quote");
  }
}

function keyTypes(tree: MessageTree | string | string[], path = ""): [string, string][] {
  const type = typeof tree === "string" ? "message" : Array.isArray(tree) ? "array" : "namespace";
  return [[path, type], ...(typeof tree === "string" ? [] : Object.entries(tree).flatMap(([key, value]) =>
    keyTypes(value, Array.isArray(tree) ? `${path}[${key}]` : path ? `${path}.${key}` : key)))];
}

export function validateCatalog(base: MessageTree, translated: MessageTree): string[] {
  const english = new Map(leaves(base));
  const other = new Map(leaves(translated));
  const errors: string[] = [];
  const baseKeys = new Map(keyTypes(base)), translatedKeys = new Map(keyTypes(translated));
  for (const [key, type] of baseKeys) {
    if (!translatedKeys.has(key)) errors.push(`${key}: missing key`);
    else if (translatedKeys.get(key) !== type) errors.push(`${key}: catalogue node type mismatch`);
  }
  for (const key of translatedKeys.keys()) if (!baseKeys.has(key)) errors.push(`${key}: extra key`);
  for (const [locale, entries] of [["base", english], ["translation", other]] as const) {
    for (const [key, message] of entries) {
      try { parse(message); } catch { errors.push(`${locale}.${key}: invalid ICU`); }
      try { checkApostrophes(message); } catch { errors.push(`${locale}.${key}: apostrophe`); }
      if (!message.trim()) errors.push(`${locale}.${key}: empty message`);
    }
  }
  for (const [key, message] of english) {
    if (!other.has(key)) continue;
    try {
      if (JSON.stringify(icuShape(message)) !== JSON.stringify(icuShape(other.get(key)!))) {
        errors.push(`${key}: ICU argument or branch mismatch`);
      }
    } catch { /* Syntax failures are reported above, including unmatched branches. */ }
  }
  return errors;
}

export function sampleArguments(message: string): Record<string, unknown> {
  const values: Record<string, unknown> = {};
  const visit = (elements: MessageFormatElement[]) => {
    for (const element of elements) {
      if (element.type === TYPE.literal || element.type === TYPE.pound) continue;
      if (element.type === TYPE.tag) {
        values[element.value] = (chunks: unknown) => chunks;
        visit(element.children);
      } else {
        values[element.value] = element.type === TYPE.number || element.type === TYPE.plural ? 2
          : element.type === TYPE.date || element.type === TYPE.time ? new Date(0) : "x";
        if (element.type === TYPE.select || element.type === TYPE.plural) {
          Object.values(element.options).forEach((option) => visit(option.value));
        }
      }
    }
  };
  visit(parse(message));
  return values;
}
