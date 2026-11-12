// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: a294ca3b35e942b8c194f9324fcd46791650dbf5
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { Document, isAlias, isMap, isNode, isScalar, isSeq, parseDocument, visit, type Node, type Pair, type Scalar, type YAMLMap } from "yaml";

export interface DynamicFile { name: string; text: string }
export type RouterCategory = "editable" | "textOnly" | "infrastructure";
export type ReferenceKind = "local" | "otherFile" | "external" | "plugin";
export interface MiddlewareReference { name: string; kind: ReferenceKind; files: string[]; types: string[] }
export interface RouterClassification {
  file: string; key: string; category: RouterCategory; reasons: string[];
  service: string; serviceUseCount: number; entryPoints: string[];
  middlewares: MiddlewareReference[];
  certificate: { role: "off" | "store" | "request" | "other"; certResolver?: string; domains?: unknown; emptyTls: boolean };
}
export interface ImportClassification {
  routers: RouterClassification[];
  services: { file: string; key: string; routerCount: number }[];
  entryPoints: string[]; resolvers: string[]; middlewareTypes: string[];
  skippedSecrets: string[];
  parseErrors: { file: string; code: string; offset?: number }[];
}
type RecordValue = Record<string, unknown>;
const record = (value: unknown): RecordValue => value !== null && typeof value === "object" && !Array.isArray(value) ? value as RecordValue : {};
const strings = (value: unknown): string[] => Array.isArray(value) ? value.filter((v): v is string => typeof v === "string") : [];
const stringList = (value: unknown) => Array.isArray(value) && value.every((v) => typeof v === "string");
const own = (value: RecordValue, key: string) => Object.hasOwn(value, key);
const same = (a: unknown, b: unknown): boolean => {
  if (Object.is(a, b)) return true;
  if (Array.isArray(a) && Array.isArray(b)) return a.length === b.length && a.every((v, i) => same(v, b[i]));
  if (a && b && typeof a === "object" && typeof b === "object") {
    const x = record(a), y = record(b);
    return Object.keys(x).length === Object.keys(y).length && Object.keys(x).every((k) => own(y, k) && same(x[k], y[k]));
  }
  return false;
};
/** Check names before accessing text; example templates are readable evidence, not secret files. */
export const isSecretFile = (name: string): boolean => /\.secret[^/]*$/i.test(name) && !/\.example$/i.test(name);
const fileReference = (name: string): string | undefined => !name.includes("@") ? name : name.endsWith("@file") ? name.slice(0, -5) : undefined;
const domain = (value: string) => value.length <= 253 && value.split(".").length > 1 && value.split(".").every((part) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i.test(part));
const knownKeys = (value: RecordValue, keys: string[]) => Object.keys(value).every((k) => keys.includes(k));
function ruleReasons(rule: unknown): string[] {
  if (typeof rule !== "string") return ["invalid-rule"];
  const match = rule.match(/^\s*(Host|HostRegexp)\(([\x60"])([^\x60"]+)\2\)(?:\s*&&\s*PathPrefix\(([\x60"])(\/[^\x60"]*)\4\))?\s*$/);
  if (!match) {
    if (/\|\||&&/.test(rule)) return ["combined-rule"];
    if (/\bPath\(/.test(rule)) return ["path-matcher"];
    return ["unsupported-matcher"];
  }
  if (match[1] === "Host") return domain(match[3]!) ? [] : ["unsupported-host"];
  const prefix = "^[a-z0-9-]+\\.", expression = match[3]!;
  const base = expression.slice(prefix.length, -1);
  return expression.startsWith(prefix) && expression.endsWith("$") && domain(base.replaceAll("\\.", ".")) && base.replaceAll("\\.", ".").replaceAll(".", "\\.") === base ? [] : ["unsupported-host-regexp"];
}
function tlsSupported(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const tls = record(value);
  if (!knownKeys(tls, ["certResolver", "domains"]) || (own(tls, "certResolver") && typeof tls.certResolver !== "string")) return false;
  if (!own(tls, "domains")) return true;
  const domains = tls.domains;
  if (!Array.isArray(domains) || domains.length !== 1 || typeof tls.certResolver !== "string") return false;
  const item = record(domains[0]);
  return knownKeys(item, ["main", "sans"]) && typeof item.main === "string" && domain(item.main) && same(item.sans, ["*." + item.main]);
}
interface ParsedFile { name: string; routers: RecordValue; services: RecordValue; middlewares: RecordValue; specialYaml: boolean }
/** This is form compatibility evidence, not a replacement for Traefik schema validation. */
export function classifyTraefikFiles(files: readonly DynamicFile[]): ImportClassification {
  const result: ImportClassification = { routers: [], services: [], entryPoints: [], resolvers: [], middlewareTypes: [], skippedSecrets: [], parseErrors: [] };
  const parsed: ParsedFile[] = [];
  for (const file of files) {
    if (isSecretFile(file.name)) { result.skippedSecrets.push(file.name); continue; }
    try {
      const doc = parseDocument(file.text, { prettyErrors: false, keepSourceTokens: true });
      if (doc.errors.length) {
        result.parseErrors.push(...doc.errors.map((e) => ({ file: file.name, code: e.code, offset: e.pos[0] })));
        continue;
      }
      let specialYaml = false;
      visit(doc, { Node: (_, node) => { if (isAlias(node) || node.anchor || node.tag) specialYaml = true; } });
      const root = record(doc.toJS({ maxAliasCount: 100 }));
      const http = record(root.http);
      parsed.push({ name: file.name, routers: record(http.routers), services: record(http.services), middlewares: record(http.middlewares), specialYaml });
    } catch {
      result.parseErrors.push({ file: file.name, code: "document-conversion" });
    }
  }
  const definitions = (kind: "services" | "middlewares", name: string) => {
    const key = fileReference(name);
    return key === undefined ? [] : parsed.filter((f) => own(f[kind], key)).map((f) => ({ file: f.name, value: record(f[kind][key]) }));
  };
  const uses = new Map<string, number>();
  for (const f of parsed) for (const router of Object.values(f.routers)) {
    const name = record(router).service;
    const key = typeof name === "string" ? fileReference(name) : undefined;
    if (key !== undefined) uses.set(key, (uses.get(key) ?? 0) + 1);
  }
  for (const f of parsed) {
    for (const key of Object.keys(f.services)) {
      result.services.push({ file: f.name, key, routerCount: uses.get(key) ?? 0 });
    }
    for (const value of Object.values(f.middlewares)) result.middlewareTypes.push(...Object.keys(record(value)));
    for (const [key, value] of Object.entries(f.routers)) {
      const router = record(value), reasons: string[] = [];
      if (!value || typeof value !== "object" || Array.isArray(value)) reasons.push("invalid-router-shape");
      reasons.push(...ruleReasons(router.rule));
      if (own(router, "priority")) reasons.push("priority");
      if (!knownKeys(router, ["rule", "entryPoints", "middlewares", "service", "tls", "priority"])) reasons.push("unknown-router-key");
      if (f.specialYaml) reasons.push("yaml-indirection");
      for (const field of ["entryPoints", "middlewares"]) if (own(router, field) && !stringList(router[field])) reasons.push("invalid-" + field);
      const entryPoints = strings(router.entryPoints);
      result.entryPoints.push(...entryPoints);
      const service = typeof router.service === "string" ? router.service : "";
      const services = definitions("services", service), serviceUseCount = uses.get(fileReference(service) ?? "") ?? 0;
      if (!services.length) reasons.push("external-service");
      if (services.length > 1) reasons.push("ambiguous-service");
      if (serviceUseCount > 1) reasons.push("shared-service");
      const backend = services[0]?.value ?? {}, lb = record(backend.loadBalancer);
      if (services.length && (!knownKeys(backend, ["loadBalancer"]) || !knownKeys(lb, ["servers"]) || !Array.isArray(lb.servers) || !lb.servers.length || lb.servers.some((v) => {
        const server = record(v);
        if (!knownKeys(server, ["url"]) || typeof server.url !== "string") return true;
        try { return !["http:", "https:"].includes(new URL(server.url).protocol); } catch { return true; }
      }))) reasons.push("unsupported-service");
      const middlewares: MiddlewareReference[] = strings(router.middlewares).map((name) => {
        const defs = definitions("middlewares", name), types = [...new Set(defs.flatMap((d) => Object.keys(d.value)))];
        return { name, kind: types.includes("plugin") ? "plugin" : defs.some((d) => d.file === f.name) ? "local" : defs.length ? "otherFile" : "external", files: defs.map((d) => d.file), types };
      });
      const tls = record(router.tls), emptyTls = own(router, "tls") && router.tls !== null && typeof router.tls === "object" && !Array.isArray(router.tls) && !Object.keys(tls).length;
      const certificate: RouterClassification["certificate"] = {
        role: !own(router, "tls") ? "off" : own(tls, "domains") || own(tls, "certResolver") ? "request" : emptyTls ? "store" : "other",
        emptyTls, ...(typeof tls.certResolver === "string" ? { certResolver: tls.certResolver } : {}), ...(own(tls, "domains") ? { domains: tls.domains } : {}),
      };
      if (certificate.certResolver) result.resolvers.push(certificate.certResolver);
      if (own(router, "tls") && !tlsSupported(router.tls)) reasons.push("unsupported-tls");
      const catchAll = typeof router.rule === "string" && /^\s*(?:PathPrefix\(([\x60"])\/\1\)|HostRegexp\(([\x60"])(?:\.\*|\^\.\*\$)\2\))\s*$/.test(router.rule);
      const redirect = middlewares.some((m) => m.types.some((type) => ["redirectScheme", "redirectRegex"].includes(type)));
      if (catchAll) reasons.push("catch-all");
      if (redirect) reasons.push("redirect");
      result.routers.push({ file: f.name, key, category: catchAll || redirect ? "infrastructure" : reasons.length ? "textOnly" : "editable", reasons, service, serviceUseCount, entryPoints, middlewares, certificate });
    }
  }
  for (const field of ["entryPoints", "resolvers", "middlewareTypes"] as const) result[field] = [...new Set(result[field])].sort();
  return result;
}

export type YamlValue = string | number | boolean | null | YamlValue[] | { [key: string]: YamlValue };
/** null removes an optional field; omitted fields stay untouched. */
export interface RouterPatch { rule?: string; entryPoints?: string[] | null; middlewares?: string[] | null; service?: string; tls?: { [key: string]: YamlValue } | null }
interface Edit { start: number; end: number; text: string; indent?: number }
function editableDocument(text: string) {
  const doc = parseDocument(text, { prettyErrors: false, keepSourceTokens: true });
  if (doc.errors.length) throw new Error("invalid-yaml");
  let indirect = false;
  visit(doc, { Node: (_, node) => { if (isAlias(node) || node.anchor || node.tag) indirect = true; } });
  // A local edit must not mutate another node through an alias or strand its anchor.
  if (indirect) throw new Error("yaml-indirection");
  return doc;
}
function mapAt(doc: Document, path: string[]): YAMLMap {
  const value = doc.getIn(path, true);
  if (!isMap(value)) throw new Error("expected-map");
  return value;
}
function nodeRange(node: Node): [number, number, number] {
  if (!node.range) throw new Error("missing-source-range");
  return node.range;
}
function applyEdits(text: string, edits: Edit[]): string {
  let boundary = text.length;
  for (const edit of edits.sort((a, b) => b.start - a.start || b.end - a.end || (a.indent ?? 0) - (b.indent ?? 0))) {
    if (edit.end > boundary) throw new Error("overlapping-edits");
    text = text.slice(0, edit.start) + edit.text + text.slice(edit.end);
    boundary = edit.start;
  }
  if (parseDocument(text, { prettyErrors: false }).errors.length) throw new Error("invalid-edit");
  return text;
}
const newline = (text: string) => text.includes("\r\n") ? "\r\n" : "\n";
const lineStart = (text: string, offset: number) => text.lastIndexOf("\n", offset - 1) + 1;
const indentAt = (text: string, offset: number) => offset - lineStart(text, offset);
function render(node: Node, indent: number, text: string): string {
  const doc = new Document();
  node.comment = undefined; node.commentBefore = undefined; node.spaceBefore = undefined;
  doc.contents = node;
  const value = doc.toString({ lineWidth: 0 }).replace(/\n$/, "");
  return value.replaceAll("\n", newline(text) + " ".repeat(indent));
}
/** Remove data, retaining literal CST comment tokens as standalone comment lines. */
function commentsIn(text: string, node: unknown, start: number, end: number, indent: number): string {
  const comments = new Map<number, string>();
  function walk(value: unknown) {
    if (!value || typeof value !== "object") return;
    const token = value as { type?: string; offset?: number; source?: string };
    if (token.type === "comment" && token.offset !== undefined && token.offset >= start && token.offset < end && token.source) comments.set(token.offset, token.source);
    for (const child of Object.values(value)) if (Array.isArray(child)) child.forEach(walk); else if (typeof child === "object") walk(child);
  }
  walk(node);
  return [...comments].sort(([a], [b]) => a - b).map(([, source]) => " ".repeat(indent) + source + newline(text)).join("");
}
function pairRemoval(text: string, pair: Pair): Edit {
  if (!isNode(pair.key) || !isNode(pair.value)) throw new Error("unsupported-entry");
  const start = lineStart(text, nodeRange(pair.key)[0]);
  const end = nodeRange(pair.value)[2];
  if (text.slice(start, nodeRange(pair.key)[0]).trim()) throw new Error("unsupported-flow-removal");
  return { start, end, text: commentsIn(text, pair.srcToken, start, end, indentAt(text, nodeRange(pair.key)[0])) };
}
function appendMap(text: string, map: YAMLMap, values: Record<string, YamlValue>, edits: Edit[]) {
  if (!Object.keys(values).length) return;
  if (map.flow) {
    const node = new Document().createNode(values);
    node.flow = true;
    const addition = render(node, 0, text).slice(1, -1).trim();
    const end = nodeRange(map)[1] - 1;
    edits.push({ start: end, end, text: (map.items.length ? ", " : " ") + addition + " " });
    return;
  }
  const last = map.items.at(-1)!;
  if (!isNode(last.value)) throw new Error("unsupported-entry");
  const end = nodeRange(last.value)[2];
  const first = map.items[0]!.key;
  if (!isNode(first)) throw new Error("unsupported-entry");
  const indent = indentAt(text, nodeRange(first)[0]);
  const doc = new Document(values);
  const addition = doc.toString({ lineWidth: 0 }).trimEnd().split("\n").map((line) => " ".repeat(indent) + line).join(newline(text)) + newline(text);
  edits.push({ start: end, end, text: (text[end - 1] === "\n" ? "" : newline(text)) + addition, indent });
}
function replaceValue(text: string, node: Node, value: YamlValue, edits: Edit[]) {
  if (same(node.toJSON(), value)) return;
  if (isMap(node) && value !== null && typeof value === "object" && !Array.isArray(value) && Object.keys(value).length && !node.flow && node.items.length) {
    const additions: Record<string, YamlValue> = {};
    for (const pair of node.items) {
      if (!isScalar(pair.key) || typeof pair.key.value !== "string" || !isNode(pair.value)) throw new Error("unsupported-entry");
      if (!Object.hasOwn(value, pair.key.value)) edits.push(pairRemoval(text, pair));
      else replaceValue(text, pair.value, value[pair.key.value]!, edits);
    }
    for (const [key, item] of Object.entries(value)) if (!node.has(key)) additions[key] = item;
    appendMap(text, node, additions, edits);
    return;
  }
  if (isSeq(node) && Array.isArray(value) && value.length && (node.items.length === value.length || !node.flow)) {
    const indent = indentAt(text, nodeRange(node)[0]);
    node.items.forEach((item, index) => {
      if (!isNode(item)) throw new Error("unsupported-entry");
      if (index < value.length) replaceValue(text, item, value[index]!, edits);
      else {
        const start = lineStart(text, nodeRange(item)[0]), end = nodeRange(item)[2];
        edits.push({ start, end, text: commentsIn(text, node.srcToken, start, end, indent) });
      }
    });
    if (value.length > node.items.length) {
      const end = nodeRange(node)[2];
      const addition = new Document(value.slice(node.items.length)).toString({ lineWidth: 0 }).trimEnd().split("\n").map((line) => " ".repeat(indent) + line).join(newline(text)) + newline(text);
      edits.push({ start: end, end, text: (text[end - 1] === "\n" ? "" : newline(text)) + addition, indent });
    }
    return;
  }
  const [start, end] = nodeRange(node);
  // Structural replacement retains comment tokens; ordinary scalar changes above
  // leave their surrounding source untouched, including quoting and inline comments.
  const retained = isScalar(node) ? "" : commentsIn(text, node.srcToken, start, end, indentAt(text, start));
  let replacement: Node;
  if (isScalar(node) && (typeof value !== "object" || value === null)) { const scalar = node.clone() as Scalar; scalar.value = value; replacement = scalar; }
  else replacement = new Document().createNode(value);
  replacement.comment = undefined; replacement.commentBefore = undefined; replacement.spaceBefore = undefined;
  // A compact replacement avoids changing indentation or consuming a trailing comment.
  if (isMap(replacement) || isSeq(replacement)) replacement.flow = true;
  const ending = retained ? newline(text) + retained + (text[end - 1] === "\n" ? "" : " ".repeat(indentAt(text, start))) : text[end - 1] === "\n" ? newline(text) : "";
  edits.push({ start, end, text: render(replacement, indentAt(text, start), text) + ending });
}
/** Apply source-range edits only; never serialize the containing document. */
export function updateRouterInFile(text: string, routerKey: string, patch: RouterPatch): string {
  const doc = editableDocument(text), router = mapAt(doc, ["http", "routers", routerKey]), edits: Edit[] = [];
  if (Object.keys(patch).some((key) => !["rule", "entryPoints", "middlewares", "service", "tls"].includes(key))) throw new Error("unsupported-patch-key");
  const additions: Record<string, YamlValue> = {};
  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined) continue;
    const pair = router.items.find((p) => isScalar(p.key) && p.key.value === key);
    if (value === null) { if (pair) edits.push(pairRemoval(text, pair)); }
    else if (pair) {
      if (!isNode(pair.value)) throw new Error("unsupported-entry");
      replaceValue(text, pair.value, value, edits);
    } else additions[key] = value;
  }
  appendMap(text, router, additions, edits);
  return applyEdits(text, edits);
}
/** Add to existing maps, or initialise absent HTTP maps without rewriting neighbours. */
export function addRouteToFile(text: string, routerKey: string, router: Record<string, YamlValue>, service?: { key: string; value: YamlValue }): string {
  const doc = editableDocument(text), edits: Edit[] = [];
  const root = mapAt(doc, []);
  const httpNode = doc.get("http", true);
  if (httpNode === undefined) {
    appendMap(text, root, { http: { routers: { [routerKey]: router }, ...(service ? { services: { [service.key]: service.value } } : {}) } }, edits);
  } else {
    const http = mapAt(doc, ["http"]);
    const additions: Record<string, YamlValue> = {};
    for (const [section, key, value] of [["routers", routerKey, router], ...(service ? [["services", service.key, service.value] as const] : [])] as const) {
      if (!http.has(section)) additions[section] = { [key]: value };
      else {
        const map = mapAt(doc, ["http", section]);
        if (map.has(key)) throw new Error("duplicate-entry");
        appendMap(text, map, { [key]: value }, edits);
      }
    }
    appendMap(text, http, additions, edits);
  }
  return applyEdits(text, edits);
}
/** Retain every service: other files and providers may still reference it. */
export function removeRouteFromFile(text: string, routerKey: string): string {
  const doc = editableDocument(text), routers = mapAt(doc, ["http", "routers"]);
  const pair = routers.items.find((p) => isScalar(p.key) && p.key.value === routerKey);
  if (!pair) throw new Error("missing-router");
  if (routers.flow) throw new Error("unsupported-flow-removal");
  if (routers.items.length === 1) {
    // Keep the map valid when removing its last data entry, and retain annotations.
    const edit = pairRemoval(text, pair);
    edit.text += " ".repeat(indentAt(text, nodeRange(routers)[0])) + "{}" + newline(text);
    return applyEdits(text, [edit]);
  }
  return applyEdits(text, [pairRemoval(text, pair)]);
}
