import type { ComposeModel } from "./model";

export type IssueField = "name" | "image" | "ports" | "mounts" | "env" | "health" | "services";

export type IssueCode =
  | "name-missing"
  | "name-format"
  | "no-services"
  | "image-missing"
  | "port-container-missing"
  | "port-container-format"
  | "port-host-format"
  | "mount-target-missing"
  | "mount-target-relative"
  | "env-key-missing"
  | "env-value-empty"
  | "health-test-missing"
  | "health-interval-format"
  | "health-retries-format";

export interface Issue {
  level: "error" | "warn";
  code: IssueCode;
  field: IssueField;
  service?: string;
  /** 1-based row number for list fields; name for env warnings. */
  ref?: string;
}

const NAME = /^[a-z0-9][a-z0-9_-]*$/;
/** Compose durations: one or more number+unit pairs, e.g. 30s, 1m30s, 500ms. */
const DURATION = /^(\d+(\.\d+)?(us|ms|s|m|h))+$/;
const CONTAINER_PORT = /^\d+(-\d+)?(\/(tcp|udp))?$/;
const HOST_PORT = /^(\d{1,3}(\.\d{1,3}){3}:)?\d+(-\d+)?$/;

/** Required-field and plausibility checks; codes are translated by the UI. */
export function validateCompose(m: ComposeModel): Issue[] {
  const out: Issue[] = [];
  if (!m.name.trim()) out.push({ level: "error", code: "name-missing", field: "name" });
  else if (!NAME.test(m.name)) out.push({ level: "warn", code: "name-format", field: "name" });
  if (!m.services.length) out.push({ level: "error", code: "no-services", field: "services" });
  for (const s of m.services) {
    if (!s.image.trim()) out.push({ level: "error", code: "image-missing", field: "image", service: s.key });
    for (const p of s.ports) {
      if (!p.editable) continue;
      const ref = String(p.index + 1);
      if (!p.container.trim()) out.push({ level: "error", code: "port-container-missing", field: "ports", service: s.key, ref });
      else if (!CONTAINER_PORT.test(p.container.trim())) out.push({ level: "warn", code: "port-container-format", field: "ports", service: s.key, ref });
      if (p.host && !HOST_PORT.test(p.host.trim())) out.push({ level: "warn", code: "port-host-format", field: "ports", service: s.key, ref });
    }
    for (const v of s.mounts) {
      if (!v.editable) continue;
      const ref = String(v.index + 1);
      if (!v.target.trim()) out.push({ level: "error", code: "mount-target-missing", field: "mounts", service: s.key, ref });
      else if (!v.target.startsWith("/")) out.push({ level: "warn", code: "mount-target-relative", field: "mounts", service: s.key, ref });
    }
    for (const e of s.env) {
      if (!e.key.trim()) out.push({ level: "error", code: "env-key-missing", field: "env", service: s.key, ref: String(e.index + 1) });
      else if (!e.value.trim()) out.push({ level: "warn", code: "env-value-empty", field: "env", service: s.key, ref: e.key });
    }
    if (s.health.enabled && !(s.health.form === "CMD" ? s.health.args[0]?.trim() : s.health.test.trim())) out.push({ level: "error", code: "health-test-missing", field: "health", service: s.key });
    if (s.health.enabled && s.health.interval.trim() && !DURATION.test(s.health.interval.trim())) out.push({ level: "error", code: "health-interval-format", field: "health", service: s.key, ref: "interval" });
    if (s.health.enabled && s.health.retries.trim() && !/^[1-9]\d*$/.test(s.health.retries.trim())) out.push({ level: "error", code: "health-retries-format", field: "health", service: s.key, ref: "retries" });
  }
  return out;
}
