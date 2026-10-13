/*
 * Compose model for the stack editor. The raw text is the single source of
 * truth; the form reads this derived model and writes through ops.ts.
 * Adapted from the Opus Arcane mockup (handoff/opus-pc/src/app/variants/opus-arcane/compose.ts,
 * no upstream commit; copied 2026-10-10). Deviations: React-neutral module, more
 * service keys (command, networks), parse errors carry line numbers as data.
 */
import { type Document, parseDocument } from "yaml";

export interface PortRow {
  index: number;
  host: string;
  container: string;
  editable: boolean;
  raw: string;
}

export interface MountRow {
  index: number;
  source: string;
  target: string;
  readOnly: boolean;
  editable: boolean;
  raw: string;
}

export interface EnvRow {
  index: number;
  key: string;
  value: string;
}

/** Compose healthcheck test form: ["CMD", …args], ["CMD-SHELL", line] or a plain string (run by the shell). */
export type HealthForm = "CMD" | "CMD-SHELL" | "string";

export interface HealthModel {
  enabled: boolean;
  /** Shell line for CMD-SHELL and text; for CMD only a space-joined summary. */
  test: string;
  /** Exact exec arguments after "CMD"; empty for the shell forms. */
  args: string[];
  form: HealthForm;
  interval: string;
  retries: string;
}

export interface ServiceModel {
  key: string;
  image: string;
  containerName: string;
  restart: string;
  command: string;
  ports: PortRow[];
  mounts: MountRow[];
  env: EnvRow[];
  envStyle: "map" | "list" | "none";
  networks: string[];
  networksEditable: boolean;
  health: HealthModel;
  extras: string[];
}

export interface ComposeModel {
  name: string;
  services: ServiceModel[];
  extras: string[];
}

export interface ParseError {
  message: string;
  line?: number;
  col?: number;
}

export type ParseResult = { ok: true; model: ComposeModel } | { ok: false; errors: ParseError[] };

type Obj = Record<string, unknown>;

export const KNOWN_SERVICE_KEYS = ["image", "container_name", "restart", "command", "ports", "volumes", "environment", "healthcheck", "networks"];
const KNOWN_ROOT_KEYS = ["name", "services"];

function isObj(v: unknown): v is Obj {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function str(v: unknown): string {
  if (v === null || v === undefined) return "";
  return typeof v === "string" ? v : String(v);
}

export function parseCompose(text: string): ParseResult {
  let doc: Document.Parsed;
  try {
    doc = parseDocument(text);
  } catch (e) {
    return { ok: false, errors: [{ message: String(e) }] };
  }
  if (doc.errors.length) {
    return {
      ok: false,
      errors: doc.errors.map((e) => {
        const pos = e.linePos?.[0];
        const first = (e.message.split("\n")[0] ?? e.message).replace(/ at line \d+, column \d+:?$/, "");
        return { message: first, line: pos?.line, col: pos?.col };
      }),
    };
  }
  let js: unknown;
  try {
    js = doc.toJS();
  } catch (e) {
    return { ok: false, errors: [{ message: String(e) }] };
  }
  if (js === null || js === undefined) js = {};
  if (!isObj(js)) return { ok: false, errors: [{ message: "root-not-map" }] };
  const raw = js["services"];
  if (raw !== null && raw !== undefined && !isObj(raw)) return { ok: false, errors: [{ message: "services-not-map" }] };
  const services: ServiceModel[] = [];
  if (isObj(raw)) {
    for (const key of Object.keys(raw)) {
      const s = raw[key];
      services.push(readService(key, isObj(s) ? s : {}));
    }
  }
  return {
    ok: true,
    model: { name: str(js["name"]), services, extras: Object.keys(js).filter((k) => !KNOWN_ROOT_KEYS.includes(k)) },
  };
}

function readService(key: string, s: Obj): ServiceModel {
  const ports: PortRow[] = [];
  const rawPorts = s["ports"];
  if (Array.isArray(rawPorts)) {
    rawPorts.forEach((p, index) => {
      if (typeof p === "string" || typeof p === "number") {
        const parts = String(p).split(":");
        const container = parts[parts.length - 1] ?? "";
        const host = parts.length > 1 ? parts.slice(0, -1).join(":") : "";
        ports.push({ index, host, container, editable: true, raw: String(p) });
      } else {
        ports.push({ index, host: "", container: "", editable: false, raw: JSON.stringify(p) });
      }
    });
  }

  const mounts: MountRow[] = [];
  const rawVols = s["volumes"];
  if (Array.isArray(rawVols)) {
    rawVols.forEach((v, index) => {
      if (typeof v !== "string") {
        mounts.push({ index, source: "", target: "", readOnly: false, editable: false, raw: JSON.stringify(v) });
        return;
      }
      const parts = v.split(":");
      if (parts.length === 1) {
        mounts.push({ index, source: "", target: parts[0] ?? "", readOnly: false, editable: true, raw: v });
      } else {
        const mode = (parts[2] ?? "").split(",");
        mounts.push({ index, source: parts[0] ?? "", target: parts[1] ?? "", readOnly: mode.includes("ro"), editable: true, raw: v });
      }
    });
  }

  const env: EnvRow[] = [];
  const rawEnv = s["environment"];
  let envStyle: ServiceModel["envStyle"] = "none";
  if (Array.isArray(rawEnv)) {
    envStyle = "list";
    rawEnv.forEach((e, index) => {
      const t = str(e);
      const eq = t.indexOf("=");
      env.push({ index, key: eq < 0 ? t : t.slice(0, eq), value: eq < 0 ? "" : t.slice(eq + 1) });
    });
  } else if (isObj(rawEnv)) {
    envStyle = "map";
    Object.keys(rawEnv).forEach((k, index) => env.push({ index, key: k, value: str(rawEnv[k]) }));
  }

  const rawNets = s["networks"];
  const networksEditable = rawNets === undefined || Array.isArray(rawNets);
  const networks = Array.isArray(rawNets) ? rawNets.map(str) : isObj(rawNets) ? Object.keys(rawNets) : [];

  const hc = s["healthcheck"];
  let health: HealthModel = { enabled: false, test: "", args: [], form: "CMD-SHELL", interval: "", retries: "" };
  if (isObj(hc) && hc["disable"] !== true) {
    const t = hc["test"];
    let test: string;
    let form: HealthForm;
    let args: string[] = [];
    if (Array.isArray(t)) {
      const arr = t.map(str);
      form = arr[0] === "CMD" ? "CMD" : "CMD-SHELL";
      if (form === "CMD") args = arr.slice(1);
      test = arr[0] === "CMD-SHELL" || arr[0] === "CMD" ? arr.slice(1).join(" ") : arr.join(" ");
    } else {
      test = str(t);
      form = t === undefined ? "CMD-SHELL" : "string";
    }
    health = { enabled: true, test, args, form, interval: str(hc["interval"]), retries: str(hc["retries"]) };
  }

  const cmd = s["command"];
  const command = Array.isArray(cmd) ? cmd.map(str).join(" ") : str(cmd);

  return {
    key,
    image: str(s["image"]),
    containerName: str(s["container_name"]),
    restart: str(s["restart"]),
    command,
    ports,
    mounts,
    env,
    envStyle,
    networks,
    networksEditable,
    health,
    extras: Object.keys(s).filter((k) => !KNOWN_SERVICE_KEYS.includes(k)),
  };
}

export function splitImage(ref: string): { repository: string; tag: string } {
  const at = ref.indexOf("@");
  const clean = at >= 0 ? ref.slice(0, at) : ref;
  const slash = clean.lastIndexOf("/");
  const colon = clean.lastIndexOf(":");
  if (colon > slash) return { repository: clean.slice(0, colon), tag: clean.slice(colon + 1) };
  return { repository: clean, tag: "latest" };
}
