// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original activity and event DTO projection for issue 38.

import type { components } from "@/api/generated/schema";
import type { ActionKind, ActionStatus, DeployStepKey, EventKind, EventLevel, OutputStream, StepState } from "@/domain/model";

import type { CodedActionRun, CodedEvent, MessageCode } from "../message-code";

type Activity = components["schemas"]["ActivityActivity"];
type Detail = components["schemas"]["ActivityDetail"];
type Event = components["schemas"]["EventEvent"];

const STEP_KEYS = new Set<DeployStepKey>(["validate", "pull", "build", "create", "stop", "start", "health", "scan", "collect", "save", "wait", "connect"]);
const STREAMS = new Set<OutputStream>(["cmd", "stdout", "stderr", "note"]);
const KINDS: Record<string, ActionKind> = {
  image_pull: "pull", image_update_check: "check-updates", image_patch: "update",
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
  return code ? { code, params: {}, sourceText } : { code: "runMessages.value", params: { value: sourceText } };
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
  const kind = KINDS[run.type] ?? "activity";
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
  return {
    id: run.id,
    kind,
    title: { code: `activity.runKind.${kind}`, params: {} },
    target: { code: "runMessages.value", params: { value: target } },
    hostId: run.environmentId,
    ...(run.resourceType === "project" && run.resourceId ? { stackId: run.resourceId } : {}),
    ...(run.resourceType === "container" && run.resourceId ? { containerId: run.resourceId } : {}),
    services: run.affectedServices ?? [],
    status: mapStatus(run.status),
    progress: run.progress ?? 0,
    startedAt: run.startedAt,
    finishedAt: run.endedAt,
    error: run.error ? message(undefined, run.error) : undefined,
    steps: (detail.stepTrace ?? []).map((step) => ({
      key: stepKey(step.name), label: message(step.code, step.name), state: stepState(step.status),
      startedAt: step.startedAt, finishedAt: step.endedAt,
    })),
    log: detail.messages?.length ? detail.messages.map((item) => message(item.code, item.message)) : run.latestMessage ? [message(run.messageCode, run.latestMessage)] : [],
    output,
    outputTruncated: detail.outputTruncated,
  };
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
    ...(event.resourceType === "project" && event.resourceId ? { stackId: event.resourceId } : {}),
  };
}
