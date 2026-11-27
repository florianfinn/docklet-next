// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original per-service deploy stage projection and behavioural coverage.

import type { ActionRun, StepState } from "@/domain/model";

export type DeployStageState = "running" | "done" | "error";
export interface DeployServicePull { layersDone: number; layersTotal: number; bytesDone?: number; bytesTotal?: number }
export interface DeployServiceRow {
  name: string;
  /** Compose declares a healthcheck (true), disables it (false) or leaves it to the image (absent). */
  check?: boolean;
  pull?: DeployServicePull;
  build?: DeployStageState;
  create?: DeployStageState;
  start?: DeployStageState;
  health?: DeployStageState;
  /** A one-shot container exited successfully while Compose waited for it. */
  completed?: boolean;
}
/** `prepared`: the producer reached deploy, so pulls and builds not observed were not needed. */
export interface DeployServices { rows: DeployServiceRow[]; truncated: boolean; prepared: boolean }
export type DeployServicesView<T> = T & { deployServices?: DeployServices };

export const DEPLOY_STAGES = ["pull", "build", "create", "start", "health"] as const;
export type DeployStage = (typeof DEPLOY_STAGES)[number];

const object = (value: unknown): Record<string, unknown> | undefined => value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
const integer = (value: unknown, max: number): value is number => typeof value === "number" && Number.isSafeInteger(value) && value >= 0 && value <= max;
const stage = (value: unknown): value is DeployStageState | undefined => value === undefined || value === "running" || value === "done" || value === "error";

/** Validates the producer's `deployServices` metadata; any invalid row hides the whole projection. */
export function parseDeployServices(metadata: unknown): DeployServices | undefined {
  const root = object(metadata);
  if (root?.v !== 1 || !Array.isArray(root.services) || root.services.length > 64 || (root.truncated !== undefined && root.truncated !== true) || (root.prepared !== undefined && root.prepared !== true)) return undefined;
  const rows: DeployServiceRow[] = [];
  for (const item of root.services) {
    const row = object(item);
    if (!row || typeof row.name !== "string" || !row.name || row.name.length > 256 || (row.check !== undefined && typeof row.check !== "boolean") || (row.completed !== undefined && row.completed !== true)) return undefined;
    if (!stage(row.build) || !stage(row.create) || !stage(row.start) || !stage(row.health)) return undefined;
    const parsed: DeployServiceRow = { name: row.name };
    if (row.check !== undefined) parsed.check = row.check;
    if (row.completed === true) parsed.completed = true;
    for (const key of ["build", "create", "start", "health"] as const) {
      const value = row[key];
      if (value === "running" || value === "done" || value === "error") parsed[key] = value;
    }
    if (row.pull !== undefined) {
      const pull = object(row.pull);
      if (!pull || !integer(pull.layersDone, 512) || !integer(pull.layersTotal, 512) || pull.layersTotal === 0 || pull.layersDone > pull.layersTotal) return undefined;
      parsed.pull = { layersDone: pull.layersDone, layersTotal: pull.layersTotal };
      if (integer(pull.bytesDone, Number.MAX_SAFE_INTEGER) && integer(pull.bytesTotal, Number.MAX_SAFE_INTEGER) && pull.bytesDone <= pull.bytesTotal) {
        parsed.pull.bytesDone = pull.bytesDone;
        parsed.pull.bytesTotal = pull.bytesTotal;
      }
    }
    rows.push(parsed);
  }
  return rows.length ? { rows, truncated: root.truncated === true, prepared: root.prepared === true } : undefined;
}

/** "interrupted": the stage was running when the run settled, so its outcome is unknown. */
export type SegmentState = StepState | "interrupted";
export interface ServiceSegment { stage: DeployStage; state: SegmentState; fraction?: number }
export type ServiceSummary =
  | { kind: "failed" } | { kind: "pulling"; fraction: number } | { kind: "building" | "creating" | "starting" | "waiting" }
  | { kind: "healthy" | "ready" | "completed" | "running" | "interrupted" | "pending" | "notReached" };

/** Share of the pull already transferred: bytes when every layer total is known, layers otherwise. */
export function pullFraction(pull: DeployServicePull): number {
  if (pull.bytesTotal) return pull.bytesDone! / pull.bytesTotal;
  return pull.layersDone / pull.layersTotal;
}

/**
 * Turns observed stages into five segments. An unobserved stage is skipped
 * when a later stage was observed, when preparation is over (pull and build)
 * or when the run succeeded; otherwise it is still pending. Work still
 * running when the run settled, and a pull that ended partway, is interrupted;
 * confirmed completion and failure are kept.
 */
export function serviceSegments(row: DeployServiceRow, run: Pick<ActionRun, "status">, prepared: boolean): ServiceSegment[] {
  const active = run.status === "running" || run.status === "queued";
  const settle = (state?: DeployStageState): SegmentState | undefined => state === "running" && !active ? "interrupted" : state;
  const observed: Record<DeployStage, SegmentState | undefined> = {
    pull: row.pull ? row.pull.layersDone === row.pull.layersTotal ? "done" : active && !prepared ? "running" : "interrupted" : undefined,
    build: settle(row.build), create: settle(row.create), start: settle(row.start),
    // A disabled healthcheck is dashed, but a confirmed failed wait stays visible.
    health: row.check === false && row.health !== "error" ? "skipped" : settle(row.health),
  };
  return DEPLOY_STAGES.map((key, index) => {
    const later = DEPLOY_STAGES.slice(index + 1).some((next) => observed[next] && observed[next] !== "skipped");
    const state = observed[key] ?? (later || run.status === "succeeded" || (prepared && (key === "pull" || key === "build")) ? "skipped" : "pending");
    return { stage: key, state, ...(key === "pull" && row.pull && (state === "running" || state === "interrupted") ? { fraction: pullFraction(row.pull) } : {}) };
  });
}

/** The row's short status: an error, the running stage, the reached end state, or an interruption. */
export function serviceSummary(row: DeployServiceRow, segments: ServiceSegment[], run: Pick<ActionRun, "status">): ServiceSummary {
  if (segments.some((s) => s.state === "error")) return { kind: "failed" };
  const running = segments.find((s) => s.state === "running");
  if (running) {
    if (running.stage === "pull") return { kind: "pulling", fraction: running.fraction ?? 0 };
    return { kind: ({ build: "building", create: "creating", start: "starting", health: "waiting" } as const)[running.stage] };
  }
  const health = segments[4]!, start = segments[3]!;
  // Completion is a confirmed wait outcome, also when a disabled healthcheck keeps the segment dashed.
  if (row.completed) return { kind: "completed" };
  if (health.state === "done") return { kind: row.check === true ? "healthy" : "ready" };
  if (start.state === "done" && health.state !== "interrupted") return { kind: "running" };
  if (segments.some((s) => s.state === "interrupted")) return { kind: "interrupted" };
  return { kind: run.status === "running" || run.status === "queued" ? "pending" : "notReached" };
}

/** The mapper supplies rows for deploy and update runs; the producer marks the end of preparation. */
export function deployServicesOf(run: ActionRun): { services: DeployServices; prepared: boolean } | undefined {
  const services = (run as DeployServicesView<ActionRun>).deployServices;
  if (!services || run.outcomeState === "unknown") return undefined;
  return { services, prepared: services.prepared || (run.status !== "running" && run.status !== "queued") };
}
