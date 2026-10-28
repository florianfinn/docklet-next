// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original activity and event DTO projection for issue 38.

import type { components } from "@/api/generated/schema";
import type { ActionKind, ActionStatus, DeployStepKey, EventKind, EventLevel, OutputStream, StepState } from "@/domain/model";

import type { CodedActionRun, CodedEvent, MessageCode } from "../message-code";
import type { RuntimeView } from "@/features/activity/runtime-presentation";
import { reason, record, bool, id, date, optionalDate } from "./update-validation";
import { inventoryId } from "./inventory-mapping";
import { backupCodeKey } from "../backup-error";
import { VULNERABILITY_CODES } from "./vulnerability-decoders";

type Activity = components["schemas"]["ActivityActivity"];
type Detail = components["schemas"]["ActivityDetail"];
type Event = components["schemas"]["EventEvent"];

const STEP_KEYS = new Set<DeployStepKey>(["validate", "pull", "build", "create", "stop", "start", "health", "scan", "collect", "save", "wait", "connect"]);
const STREAMS = new Set<OutputStream>(["cmd", "stdout", "stderr", "note"]);
const KINDS: Record<string, ActionKind> = {
  image_pull: "pull", image_update_check: "check-updates", image_patch: "update",
  project_update: "update", project_restore: "restore",
  project_deploy: "deploy", project_redeploy: "redeploy", project_down: "stop",
  project_restart: "restart", container_start: "start", container_stop: "stop",
  container_restart: "restart", container_redeploy: "redeploy", container_edit: "recreate",
  vulnerability_scan: "scan", system_prune: "prune", auto_update: "update",
};

function required(value: unknown, field: string): asserts value {
  if (value === undefined || value === null || value === "") throw new Error(`dto.invalid-${field}`);
}

export function mapStatus(value: string): ActionStatus {
  switch (value) {
    case "queued": return "queued";
    case "running": return "running";
    case "success": return "succeeded";
    case "failed": return "failed";
    case "cancelled": return "cancelled";
    default: throw new Error("dto.invalid-status");
  }
}

function stepState(status: string): StepState {
  switch (mapStatus(status)) {
    case "queued": return "pending";
    case "running": return "running";
    case "succeeded": return "done";
    case "failed": return "error";
    case "cancelled": return "skipped";
  }
}

function message(code: string | undefined, sourceText: string): MessageCode {
  return code ? { code, params: code === "activity.message" ? { message: sourceText } : {}, sourceText } : { code: "runMessages.value", params: { value: sourceText } };
}

/** Scan activities persist declared vulnerability codes as their error; render the catalogue text. */
function vulnerabilityError(error: string): MessageCode | undefined {
  return (VULNERABILITY_CODES as readonly string[]).includes(error) ? { code: `images.errors.${error.replaceAll(".", "_")}`, params: {} } : undefined;
}

function stepKey(name: string): DeployStepKey {
  const key = name.toLowerCase().replaceAll(/[^a-z]+/g, "") as DeployStepKey;
  return STEP_KEYS.has(key) ? key : "other";
}

export function mapActivityDetail(detail: Detail): CodedActionRun {
  if (!detail || !detail.activity) throw new Error("dto.invalid-detail");
  const run: Activity = detail.activity;
  for (const [key, value] of Object.entries({ id: run.id, type: run.type, environmentId: run.environmentId, startedAt: run.startedAt })) required(value, key);
  if (typeof detail.outputTruncated !== "boolean") throw new Error("dto.invalid-output-truncation");
  if ((run.outcomeState !== undefined || run.cancelRequested !== undefined) &&
      ((run.outcomeState !== "known" && run.outcomeState !== "unknown") || typeof run.cancelRequested !== "boolean")) throw new Error("dto.invalid-runtime-knowledge");
  const action = typeof run.metadata?.action === "string" ? run.metadata.action : "";
  const backup = /volume_backup/.test(action) || run.resourceType === "backup_target" || run.resourceType === "volume_backup_download";
  const kind: ActionKind = backup ? action.startsWith("restore_") ? "volume-restore" : "backup" : KINDS[run.type] ?? "activity";
  const lifecycle = run.imageLifecycle;
  const m10 = run.type === "project_update" || run.type === "project_restore" || !!lifecycle;
  if (m10) { date(run.startedAt); optionalDate(run.endedAt); }
  const coded = (code: string | undefined, source: string, field: "progress" | "error" = "progress"): MessageCode => {
    if (backup) {
      const key = backupCodeKey(code) ?? backupCodeKey(source);
      if (key) return { code: key, params: {} };
      return { code: field === "error" ? "common.commandErrors.failed" : `activity.runKind.${kind}`, params: {} };
    }
    if (!m10) return message(code, source);
    const safe = reason(code);
    if (safe && safe !== "http.error") return { code: `images.errors.${safe.replaceAll(".", "_")}`, params: {} };
    const keys: Record<string, string> = { "run.started": "activity.status.running", "run.joined": "activity.status.running", "runtime.cancelled": "activity.runtime.cancelledBody", "runtime.failed": "activity.runtime.failedBody" };
    const key = keys[code ?? ""];
    if (key) return { code: key, params: {} };
    return field === "error" ? { code: "common.commandErrors.failed", params: {} } : message(code, source);
  };
  if (lifecycle) { const l = record(lifecycle); id(l.runId); bool(l.verified); if (bool(l.volumeDataIncluded)) throw new Error("dto.invalid-lifecycle"); }
  const target = run.resourceName || run.resourceId || run.sourceEnvironmentName || run.type;
  const output = (detail.output ?? []).map((line) => {
    required(line.timestamp, "output-timestamp");
    if (!STREAMS.has(line.stream as OutputStream) || typeof line.text !== "string") throw new Error("dto.invalid-output-line");
    return {
      at: line.timestamp,
      stream: line.stream as OutputStream,
      text: line.text,
      ...(line.step ? { step: stepKey(line.step) } : {}),
      ...(line.level === "error" ? { level: "error" as const } : line.level === "warning" ? { level: "warn" as const } : {}),
    };
  });
  return Object.assign({
    id: run.id,
    ...(lifecycle ? { baselineCode: reason(lifecycle.baselineCode), lifecycleCode: reason(lifecycle.code), effectVerified: lifecycle.verified, restorePointId: lifecycle.restorePointId } : {}),
    kind,
    title: { code: `activity.runKind.${kind}`, params: {} },
    target: { code: "runMessages.value", params: { value: target } },
    hostId: run.environmentId,
    ...((run.resourceType === "project" || run.resourceType === "stack") && run.resourceId ? { stackId: inventoryId(run.environmentId, run.resourceId) } : {}),
    ...(run.resourceType === "container" && run.resourceId ? { containerId: inventoryId(run.environmentId, run.resourceId) } : {}),
    services: run.affectedServices ?? [],
    status: mapStatus(run.status),
    ...(run.outcomeState !== undefined ? { outcomeState: run.outcomeState, cancelRequested: run.cancelRequested } : {}),
    progress: run.progress ?? 0,
    startedAt: run.startedAt,
    finishedAt: run.endedAt,
    error: run.error ? (backup ? coded(run.error, "", "error") : vulnerabilityError(run.error)) ?? coded(lifecycle?.code ?? (run.messageCode?.startsWith("runtime.") ? run.messageCode : undefined), run.error, "error") : undefined,
    steps: (detail.stepTrace ?? []).map((step) => ({
      key: stepKey(step.name), label: coded(step.code, step.name), state: stepState(step.status),
      startedAt: step.startedAt, finishedAt: step.endedAt,
    })),
    log: detail.messages?.length ? detail.messages.map((item) => coded(item.code, item.message)) : run.latestMessage ? [coded(run.messageCode, run.latestMessage)] : [],
    output,
    outputTruncated: detail.outputTruncated,
  }, { summary: { stepsAvailable: true } });
}

/** Summary mapping shares validation without retaining detail histories. */
export function mapActivitySummary(item: components["schemas"]["ActivityActivitySummary"]): CodedActionRun {
  const run: RuntimeView<CodedActionRun> = mapActivityDetail({ activity: item, stepTrace: item.stepTrace ?? [], messages: [], output: [], outputTruncated: false });
  Object.assign(run, { summary: { stepsAvailable: item.stepTrace !== undefined && item.stepTrace !== null } });
  run.log = [];
  if (run.outcomeState !== undefined) run.runtime = {
    released: item.recovery?.disposition === "released",
    ...(item.recovery?.unknownReason ? { unknownReason: item.recovery.unknownReason } : {}),
    resultCode: item.messageCode,
    sourceObserved: true,
  };
  return run;
}

export function summaryOnly(run: CodedActionRun): CodedActionRun {
  const presentation = (run as RuntimeView<CodedActionRun>).runtime;
  const minimal = presentation ? Object.fromEntries(Object.entries({ released: presentation.released,
    unknownReason: presentation.unknownReason, resultCode: presentation.resultCode,
    currentIntent: presentation.currentIntent, sourceObserved: presentation.sourceObserved }).filter(([, value]) => value !== undefined)) : undefined;
  return { ...run, log: [], output: [], outputTruncated: false, ...(minimal ? { runtime: minimal } : {}) };
}

/** A later summary owns status; the selected detail retains its separate history. */
export function detailWithSummary(summary: CodedActionRun, detail: CodedActionRun): RuntimeView<CodedActionRun> {
  return { ...detail, ...summary, output: detail.output, log: detail.log, outputTruncated: detail.outputTruncated,
    runtime: { ...(detail as RuntimeView<CodedActionRun>).runtime, ...(summary as RuntimeView<CodedActionRun>).runtime } };
}

function eventKind(event: Event): EventKind {
  const type = event.type.toLowerCase();
  if (type.includes("deploy") || type.startsWith("project.")) return "deploy";
  if (type.includes("update")) return "update";
  if (type.includes("restart")) return "restart";
  if (type.includes("health")) return "health";
  if (type.includes("backup")) return "backup";
  if (type.includes("scan")) return "scan";
  if (type.includes("environment") || type.includes("host")) return "host";
  return "activity";
}

function eventLevel(severity: string): EventLevel {
  switch (severity) {
    case "info": return "info";
    case "success": return "ok";
    case "warning": return "warn";
    case "error": return "error";
    default: throw new Error("dto.invalid-severity");
  }
}

export function mapEvent(event: Event): CodedEvent {
  for (const [key, value] of Object.entries({ id: event?.id, type: event?.type, timestamp: event?.timestamp, messageCode: event?.messageCode })) required(value, key);
  return {
    id: event.id, at: event.timestamp, kind: eventKind(event), level: eventLevel(event.severity),
    title: message(event.messageCode, event.title),
    hostId: event.environmentId,
    ...(event.resourceType === "project" && event.resourceId && event.environmentId ? { stackId: inventoryId(event.environmentId, event.resourceId) } : {}),
  };
}
