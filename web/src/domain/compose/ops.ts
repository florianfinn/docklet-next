/*
 * Targeted Compose mutations through the yaml Document API, so comments, key
 * order and unknown fields survive form edits. Adapted from the Opus Arcane
 * mockup compose.ts (copied 2026-10-10); command and networks ops are new.
 */
import { type Document, isMap, isScalar, isSeq, parseDocument, Scalar } from "yaml";

import type { HealthPreset } from "./health-presets";
import type { HealthForm } from "./model";

type Obj = Record<string, unknown>;

export type Mutator = (doc: Document.Parsed) => void;

export class ComposeEditError extends Error {
  readonly code: "invalid-yaml" | "duplicate-service" | "declaration-conflict";
  readonly detail?: string;
  constructor(code: ComposeEditError["code"], detail?: string) {
    super(code);
    this.code = code;
    this.detail = detail;
    this.name = "ComposeEditError";
  }
}

/** Applies a form edit to raw text; throws on invalid YAML so nothing is lost. */
export function mutate(text: string, fn: Mutator): string {
  const doc = parseDocument(text);
  if (doc.errors.length) throw new ComposeEditError("invalid-yaml");
  if (doc.contents === null) doc.contents = doc.createNode({}) as never;
  fn(doc);
  return doc.toString({ lineWidth: 0 });
}

const svcPath = (key: string, ...rest: (string | number)[]) => ["services", key, ...rest];

function quoted(doc: Document.Parsed, value: string): Scalar {
  const node = doc.createNode(value) as unknown as Scalar;
  node.type = Scalar.QUOTE_DOUBLE;
  return node;
}

function ensureSeq(doc: Document.Parsed, path: (string | number)[]): void {
  if (!isSeq(doc.getIn(path, true))) doc.setIn(path, doc.createNode([]));
}

function dropIfEmpty(doc: Document.Parsed, path: (string | number)[]): void {
  const node = doc.getIn(path, true);
  if ((isSeq(node) || isMap(node)) && node.items.length === 0) doc.deleteIn(path);
}

export const ops = {
  setName: (v: string): Mutator => (doc) => {
    doc.set("name", v);
  },

  setField: (key: string, field: "image" | "container_name" | "restart", v: string): Mutator => (doc) => {
    if (!v && field !== "image") doc.deleteIn(svcPath(key, field));
    else doc.setIn(svcPath(key, field), v);
  },

  setCommand: (key: string, v: string): Mutator => (doc) => {
    if (!v.trim()) doc.deleteIn(svcPath(key, "command"));
    else doc.setIn(svcPath(key, "command"), v);
  },

  renameService: (oldKey: string, newKey: string): Mutator => (doc) => {
    const services = doc.get("services", true);
    if (!isMap(services)) return;
    if (services.items.some((p) => isScalar(p.key) && p.key.value === newKey)) {
      throw new ComposeEditError("duplicate-service", newKey);
    }
    const pair = services.items.find((p) => isScalar(p.key) && p.key.value === oldKey);
    if (pair && isScalar(pair.key)) pair.key.value = newKey;
  },

  addService: (key: string, body: Obj): Mutator => (doc) => {
    if (!isMap(doc.get("services", true))) doc.set("services", doc.createNode({}));
    doc.setIn(["services", key], doc.createNode(body));
  },

  /** Adds a service with its top-level networks/volumes; a differing existing declaration refuses the whole edit. */
  importService: (key: string, body: Obj, declarations: Partial<Record<"networks" | "volumes", Obj>>): Mutator => (doc) => {
    const pending: [string[], unknown][] = [];
    for (const [section, entries] of Object.entries(declarations)) {
      for (const [name, value] of Object.entries(entries ?? {})) {
        const current = doc.getIn([section, name]);
        const plain = current && typeof current === "object" && "toJSON" in current ? (current as { toJSON(): unknown }).toJSON() : current;
        if (doc.hasIn([section, name])) {
          if (JSON.stringify(plain ?? null) !== JSON.stringify(value ?? null)) throw new ComposeEditError("declaration-conflict", name);
        } else pending.push([[section, name], value]);
      }
    }
    if (!isMap(doc.get("services", true))) doc.set("services", doc.createNode({}));
    doc.setIn(["services", key], doc.createNode(body));
    for (const [path, value] of pending) {
      if (!isMap(doc.get(path[0]!, true))) doc.set(path[0]!, doc.createNode({}));
      doc.setIn(path, value === null || value === undefined ? null : doc.createNode(value));
    }
  },

  removeService: (key: string): Mutator => (doc) => {
    doc.deleteIn(["services", key]);
  },

  addPort: (key: string): Mutator => (doc) => {
    ensureSeq(doc, svcPath(key, "ports"));
    doc.addIn(svcPath(key, "ports"), quoted(doc, ""));
  },

  setPort: (key: string, index: number, host: string, container: string): Mutator => (doc) => {
    const value = host.trim() ? `${host.trim()}:${container.trim()}` : container.trim();
    doc.setIn(svcPath(key, "ports", index), quoted(doc, value));
  },

  removePort: (key: string, index: number): Mutator => (doc) => {
    doc.deleteIn(svcPath(key, "ports", index));
    dropIfEmpty(doc, svcPath(key, "ports"));
  },

  addMount: (key: string): Mutator => (doc) => {
    ensureSeq(doc, svcPath(key, "volumes"));
    doc.addIn(svcPath(key, "volumes"), ":/data");
  },

  setMount: (key: string, index: number, source: string, target: string, readOnly: boolean): Mutator => (doc) => {
    const base = source.trim() ? `${source.trim()}:${target.trim()}` : target.trim();
    doc.setIn(svcPath(key, "volumes", index), readOnly && source.trim() ? `${base}:ro` : base);
  },

  removeMount: (key: string, index: number): Mutator => (doc) => {
    doc.deleteIn(svcPath(key, "volumes", index));
    dropIfEmpty(doc, svcPath(key, "volumes"));
  },

  addEnv: (key: string): Mutator => (doc) => {
    const path = svcPath(key, "environment");
    const env = doc.getIn(path, true);
    if (isSeq(env)) {
      doc.addIn(path, `VARIABLE_${env.items.length + 1}=`);
    } else if (isMap(env)) {
      let n = env.items.length + 1;
      while (env.has(`VARIABLE_${n}`)) n++;
      env.set(`VARIABLE_${n}`, "");
    } else {
      doc.setIn(path, doc.createNode({ VARIABLE_1: "" }));
    }
  },

  setEnv: (key: string, index: number, name: string, value: string): Mutator => (doc) => {
    const env = doc.getIn(svcPath(key, "environment"), true);
    if (isSeq(env)) {
      env.set(index, `${name}=${value}`);
    } else if (isMap(env)) {
      const pair = env.items[index];
      if (!pair) return;
      if (isScalar(pair.key)) pair.key.value = name;
      else pair.key = doc.createNode(name);
      if (isScalar(pair.value)) pair.value.value = value;
      else pair.value = doc.createNode(value);
    }
  },

  removeEnv: (key: string, index: number): Mutator => (doc) => {
    const path = svcPath(key, "environment");
    const env = doc.getIn(path, true);
    if (isSeq(env)) env.delete(index);
    else if (isMap(env)) env.items.splice(index, 1);
    dropIfEmpty(doc, path);
  },

  setNetworks: (key: string, networks: string[]): Mutator => (doc) => {
    const clean = networks.map((n) => n.trim()).filter(Boolean);
    if (!clean.length) doc.deleteIn(svcPath(key, "networks"));
    else doc.setIn(svcPath(key, "networks"), doc.createNode(clean, { flow: true }));
  },

  setHealthEnabled: (key: string, on: boolean): Mutator => (doc) => {
    const path = svcPath(key, "healthcheck");
    if (!on) {
      doc.deleteIn(path);
      return;
    }
    const hc = doc.getIn(path, true);
    if (isMap(hc)) {
      hc.delete("disable");
      return;
    }
    doc.setIn(path, doc.createNode({ test: doc.createNode(["CMD-SHELL", ""], { flow: true }), interval: "30s", retries: 3 }));
  },

  /** Shell forms only: a plain string stays a string, otherwise ["CMD-SHELL", line]. */
  setHealthTest: (key: string, cmd: string, form: Exclude<HealthForm, "CMD"> = "CMD-SHELL"): Mutator => (doc) => {
    const path = svcPath(key, "healthcheck", "test");
    if (form === "string") doc.setIn(path, cmd);
    else doc.setIn(path, doc.createNode(["CMD-SHELL", cmd], { flow: true }));
  },

  /** Exec form: writes the arguments exactly, including spaces, quotes and empty strings. */
  setHealthArgs: (key: string, args: readonly string[]): Mutator => (doc) => {
    doc.setIn(svcPath(key, "healthcheck", "test"), doc.createNode(["CMD", ...args], { flow: true }));
  },

  /** Replaces test and timing with a preset; other healthcheck keys (comments, unknown fields) stay. */
  applyHealthPreset: (key: string, p: HealthPreset): Mutator => (doc) => {
    const path = svcPath(key, "healthcheck");
    if (!isMap(doc.getIn(path, true))) doc.setIn(path, doc.createNode({}));
    const hc = doc.getIn(path, true);
    if (isMap(hc)) hc.delete("disable");
    doc.setIn([...path, "test"], doc.createNode([p.form, ...p.test], { flow: true }));
    doc.setIn([...path, "interval"], p.interval);
    doc.setIn([...path, "timeout"], p.timeout);
    doc.setIn([...path, "retries"], p.retries);
    if (p.startPeriod) doc.setIn([...path, "start_period"], p.startPeriod);
  },

  setHealthField: (key: string, field: "interval" | "retries", v: string): Mutator => (doc) => {
    const path = svcPath(key, "healthcheck", field);
    if (!v.trim()) doc.deleteIn(path);
    else doc.setIn(path, field === "retries" && /^\d+$/.test(v) ? Number(v) : v);
  },
};

export function uniqueKey(existing: string[], base: string): string {
  const clean =
    (base || "service")
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, "-")
      .replace(/^-+|-+$/g, "") || "service";
  if (!existing.includes(clean)) return clean;
  let n = 2;
  while (existing.includes(`${clean}-${n}`)) n++;
  return `${clean}-${n}`;
}
