// Source: docklet-next/issues/635
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M42 container health projection and probe reason rules.

import type { Health } from "./model";

/** One healthcheck probe as Docker reports it (the daemon keeps the last five, output capped at 4 KiB). */
export interface HealthProbe {
  start?: string;
  end?: string;
  exitCode: number;
  output: string;
}

/** Effective healthcheck configuration; durations in milliseconds, 0 means the Docker default. */
export interface HealthCheckConfig {
  test: string[];
  intervalMs: number;
  timeoutMs: number;
  retries: number;
  startPeriodMs: number;
  startIntervalMs: number;
}

/** Health projection of one container inspect; nothing else of the inspect body is kept. */
export interface ContainerHealthDetails {
  /** Inventory id (`host~dockerId`). */
  containerId: string;
  running: boolean;
  startedAt?: string;
  status: Health;
  failingStreak: number;
  probes: HealthProbe[];
  /** Absent when neither the image nor the container defines a healthcheck. */
  check?: HealthCheckConfig;
}

export interface ContainerHealthApi {
  getContainerHealth(containerId: string): Promise<ContainerHealthDetails>;
}

export type HealthReason =
  | "not-running"
  | "no-healthcheck"
  | "disabled"
  | "pending"
  | "pending-overdue"
  | "healthy"
  | "timeout"
  | "tool-missing"
  | "http-status"
  | "connection-refused"
  | "failing";

export interface HealthDiagnosis {
  reason: HealthReason;
  /** HTTP status printed by the probe tool. */
  httpStatus?: number;
  /** Executable the probe could not start. */
  tool?: string;
  /** The probe the reason was read from. */
  probe?: HealthProbe;
}

/** Docker defaults for unset (zero) healthcheck durations. */
export const HEALTH_DEFAULTS = { intervalMs: 30_000, timeoutMs: 30_000, startIntervalMs: 5_000, retries: 3 } as const;
/** Grace on top of the expected first probe before the cautious overdue hint. */
export const OVERDUE_SLACK_MS = 30_000;

const time = (value: string | undefined) => (value ? Date.parse(value) : Number.NaN);

/** RFC 3339 time as [whole-second ms, nanoseconds], so Docker's nanosecond order survives; undefined when unreadable. */
export function nanoTime(value: string | undefined): [number, number] | undefined {
  const m = value ? /^(.*T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value) : null;
  const seconds = m ? Date.parse(m[1]! + m[3]!) : Number.NaN;
  return Number.isFinite(seconds) ? [seconds, Number((m![2] ?? "").padEnd(9, "0"))] : undefined;
}

/** Probes of the current run only: a restart keeps the earlier run's entries, and an unreadable start never counts. */
function currentProbes(d: ContainerHealthDetails): HealthProbe[] {
  const started = nanoTime(d.startedAt);
  if (!started) return [];
  return d.probes.filter((p) => {
    const at = nanoTime(p.start);
    return !!at && (at[0] > started[0] || (at[0] === started[0] && at[1] >= started[1]));
  });
}

/** Latest moment the first probe should have finished: one (start) interval plus its timeout. */
export function firstProbeDue(check: HealthCheckConfig, startedAt: string): number {
  const interval = check.startPeriodMs > 0 ? check.startIntervalMs || HEALTH_DEFAULTS.startIntervalMs : check.intervalMs || HEALTH_DEFAULTS.intervalMs;
  return time(startedAt) + interval + (check.timeoutMs || HEALTH_DEFAULTS.timeoutMs);
}

const TIMEOUT = /Health check exceeded timeout/;
const TOOL_EXEC = /exec: "([^"]+)": executable file not found/;
const TOOL_SHELL = /(?:^|\n)(?:\S*\/)?(?:sh|ash|bash|dash)(?::\s*(?:line\s*)?\d+)?:\s*([^\s:]+):\s*(?:command\s+)?not found/;
const HTTP_WGET = /server returned error: HTTP\/[\d.]+ (\d{3})/;
const HTTP_CURL = /returned error: (\d{3})/;

/** Reads the reason of one failed probe from its printed output; unknown output stays "failing". */
export function classifyProbe(probe: HealthProbe): HealthDiagnosis {
  const out = probe.output;
  if (TIMEOUT.test(out)) return { reason: "timeout", probe };
  const exec = TOOL_EXEC.exec(out);
  if (exec) return { reason: "tool-missing", tool: exec[1], probe };
  const shell = TOOL_SHELL.exec(out);
  if (shell && probe.exitCode === 127) return { reason: "tool-missing", tool: shell[1], probe };
  const http = HTTP_WGET.exec(out) ?? HTTP_CURL.exec(out);
  if (http) return { reason: "http-status", httpStatus: Number(http[1]), probe };
  if (/connection refused/i.test(out)) return { reason: "connection-refused", probe };
  return { reason: "failing", probe };
}

/** Configuration and state decide first, then the newest probe of the current run. */
export function diagnoseHealth(d: ContainerHealthDetails, now: number): HealthDiagnosis {
  if (!d.running) return { reason: "not-running" };
  if (!d.check || d.check.test.length === 0) return { reason: "no-healthcheck" };
  if (d.check.test[0] === "NONE") return { reason: "disabled" };
  if (d.status === "healthy") return { reason: "healthy" };
  const probes = currentProbes(d);
  const newest = probes.at(-1);
  if (newest && newest.exitCode !== 0) return classifyProbe(newest);
  if (d.status === "unhealthy") {
    const failed = [...probes].reverse().find((p) => p.exitCode !== 0);
    return failed ? classifyProbe(failed) : { reason: "failing" };
  }
  if (!newest && d.startedAt && now > firstProbeDue(d.check, d.startedAt) + OVERDUE_SLACK_MS) return { reason: "pending-overdue" };
  return { reason: "pending" };
}

/** Probe command as one readable line; CMD-SHELL shows only the shell text. */
export function probeCommand(check: HealthCheckConfig): string {
  const [form, ...rest] = check.test;
  if (form === "CMD-SHELL") return rest.join(" ");
  if (form === "CMD") return rest.map((a) => (/[\s"']/.test(a) || a === "" ? JSON.stringify(a) : a)).join(" ");
  return check.test.join(" ");
}
