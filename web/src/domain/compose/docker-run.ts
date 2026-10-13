/*
 * docker run → Compose service conversion. Adapted from the Opus Arcane mockup
 * compose.ts (copied 2026-10-10); hints are codes so the UI can translate them.
 */
import { uniqueKey } from "./ops";

type Obj = Record<string, unknown>;

export type RunHintCode =
  | "not-docker-run"
  | "env-no-value"
  | "network-external"
  | "rm-ignored"
  | "privileged"
  | "env-file"
  | "flag-ignored"
  | "no-image"
  | "no-restart"
  | "no-ports";

export interface RunHint {
  code: RunHintCode;
  value?: string;
}

export interface Conversion {
  key: string;
  body: Obj;
  hints: RunHint[];
}

/* Shell operators outside quotes end the supported part of the input. */
const OPERATOR = /^(\|\|?|&&?|;|>>?|<|\$\(|`)/;

/** Length of a backslash-newline continuation at `i`, else 0. */
function continuation(src: string, i: number): number {
  if (src[i] !== "\\") return 0;
  return src[i + 1] === "\n" ? 2 : src[i + 1] === "\r" && src[i + 2] === "\n" ? 3 : 0;
}

/**
 * Splits a command line into arguments: single quotes are literal, in double
 * quotes only \" \\ \` \$ escape, outside quotes \x is x. `rest` is the
 * unparsed tail starting at an unsupported operator or an unclosed quote.
 */
function tokenize(input: string): { tokens: string[]; rest: string } {
  const src = input;
  const tokens: string[] = [];
  let cur = "";
  let has = false;
  let i = 0;
  const flush = () => {
    if (has) tokens.push(cur);
    cur = "";
    has = false;
  };
  while (i < src.length) {
    const ch = src[i]!;
    if (ch === "'" || ch === '"') {
      const start = i;
      i++;
      has = true;
      let closed = false;
      while (i < src.length) {
        const c = src[i]!;
        if (c === ch) {
          closed = true;
          i++;
          break;
        }
        if (ch === '"' && c === "\\" && continuation(src, i)) {
          i += continuation(src, i);
        } else if (ch === '"' && c === "\\" && i + 1 < src.length && '"\\`$'.includes(src[i + 1]!)) {
          cur += src[i + 1];
          i += 2;
        } else {
          cur += c;
          i++;
        }
      }
      if (!closed) return { tokens, rest: src.slice(start) };
    } else if (ch === "\\" && continuation(src, i)) {
      i += continuation(src, i);
    } else if (ch === "\\" && i + 1 < src.length) {
      cur += src[i + 1];
      has = true;
      i += 2;
    } else if (/\s/.test(ch)) {
      flush();
      i++;
    } else if (OPERATOR.test(src.slice(i))) {
      flush();
      return { tokens, rest: src.slice(i) };
    } else {
      cur += ch;
      has = true;
      i++;
    }
  }
  flush();
  return { tokens, rest: "" };
}

const VALUE_FLAGS: Record<string, string> = {
  "--name": "name",
  "-p": "port",
  "--publish": "port",
  "-v": "volume",
  "--volume": "volume",
  "-e": "env",
  "--env": "env",
  "--restart": "restart",
  "--network": "network",
  "--net": "network",
  "-l": "label",
  "--label": "label",
  "--health-cmd": "hcmd",
  "--health-interval": "hint",
  "--health-retries": "hretries",
  "-u": "user",
  "--user": "user",
  "-h": "hostname",
  "--hostname": "hostname",
  "-w": "workdir",
  "--workdir": "workdir",
  "--env-file": "envfile",
  "-m": "memory",
  "--memory": "memory",
};
/* Value-taking flags Docklet does not map; their value must not be read as the image. */
const IGNORED_VALUE_FLAGS = new Set([
  "--cap-add",
  "--cap-drop",
  "--device",
  "--dns",
  "--add-host",
  "--log-driver",
  "--log-opt",
  "--shm-size",
  "--ulimit",
  "--sysctl",
  "--tmpfs",
  "--entrypoint",
  "--pull",
  "--platform",
  "--security-opt",
  "--stop-signal",
  "--cpus",
  "--pid",
  "--ipc",
  "--group-add",
  "--mount",
  "--gpus",
]);
const BOOL_FLAGS = ["-d", "--detach", "--rm", "-i", "-t", "-it", "-ti", "--init", "--privileged", "--interactive", "--tty"];

export class DockerRunError extends Error {
  constructor() {
    super("not-docker-run");
    this.name = "DockerRunError";
  }
}

export function convertDockerRun(input: string, existing: string[]): Conversion {
  const { tokens: t, rest } = tokenize(input);
  if (t[0] === "sudo") t.shift();
  if (t[0] === "docker") t.shift();
  if (t[0] === "container") t.shift();
  if (t[0] !== "run") throw new DockerRunError();
  t.shift();

  const hints: RunHint[] = [];
  const ports: string[] = [];
  const vols: string[] = [];
  const env: Obj = {};
  const labels: string[] = [];
  const hc: Obj = {};
  const extra: Obj = {};
  let name = "";
  let restart = "";
  let i = 0;
  while (i < t.length) {
    let tok = t[i] ?? "";
    if (!tok.startsWith("-")) break;
    let val: string | undefined;
    if (tok.startsWith("--") && tok.includes("=")) {
      val = tok.slice(tok.indexOf("=") + 1);
      tok = tok.slice(0, tok.indexOf("="));
    }
    const kind = VALUE_FLAGS[tok];
    if (kind) {
      if (val === undefined) {
        i++;
        val = t[i] ?? "";
      }
      switch (kind) {
        case "name":
          name = val;
          break;
        case "port":
          ports.push(val);
          break;
        case "volume":
          vols.push(val);
          break;
        case "env": {
          const eq = val.indexOf("=");
          env[eq < 0 ? val : val.slice(0, eq)] = eq < 0 ? "" : val.slice(eq + 1);
          if (eq < 0) hints.push({ code: "env-no-value", value: val });
          break;
        }
        case "restart":
          restart = val;
          break;
        case "network":
          if (val === "host") extra["network_mode"] = "host";
          else {
            extra["networks"] = [val];
            hints.push({ code: "network-external", value: val });
          }
          break;
        case "label":
          labels.push(val);
          break;
        case "hcmd":
          hc["test"] = ["CMD-SHELL", val];
          break;
        case "hint":
          hc["interval"] = val;
          break;
        case "hretries":
          hc["retries"] = Number(val) || val;
          break;
        case "user":
          extra["user"] = val;
          break;
        case "hostname":
          extra["hostname"] = val;
          break;
        case "workdir":
          extra["working_dir"] = val;
          break;
        case "memory":
          extra["mem_limit"] = val;
          break;
        case "envfile":
          extra["env_file"] = [val];
          hints.push({ code: "env-file", value: val });
          break;
      }
    } else if (BOOL_FLAGS.includes(tok) || /^-[dit]+$/.test(tok)) {
      if (tok === "--rm") hints.push({ code: "rm-ignored" });
      if (tok === "--privileged") {
        extra["privileged"] = true;
        hints.push({ code: "privileged" });
      }
    } else if (IGNORED_VALUE_FLAGS.has(tok)) {
      if (val === undefined) i++;
      hints.push({ code: "flag-ignored", value: `${tok} ${val ?? t[i] ?? ""}`.trim() });
    } else {
      hints.push({ code: "flag-ignored", value: tok });
    }
    i++;
  }
  const image = t[i] ?? "";
  const command = t.slice(i + 1);
  if (rest) hints.push({ code: "flag-ignored", value: rest });
  if (!image) hints.push({ code: "no-image" });
  if (!restart) hints.push({ code: "no-restart" });
  if (!ports.length) hints.push({ code: "no-ports" });

  const body: Obj = { image };
  if (name) body["container_name"] = name;
  if (restart) body["restart"] = restart;
  if (ports.length) body["ports"] = ports;
  if (vols.length) body["volumes"] = vols;
  if (Object.keys(env).length) body["environment"] = env;
  if (labels.length) body["labels"] = labels;
  if (Object.keys(hc).length) body["healthcheck"] = hc;
  if (command.length) body["command"] = command;
  Object.assign(body, extra);

  const base = name || (image.split("/").pop() ?? "").split(":")[0] || "service";
  return { key: uniqueKey(existing, base), body, hints };
}
