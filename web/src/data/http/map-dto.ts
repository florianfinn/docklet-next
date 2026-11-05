// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original activity and event DTO projection for issue 38; catalogued labels for hub job runs.

import { operationReasonKey } from "../operation-reason";
import type { components } from "@/api/generated/schema";
import type { ActionKind, ActionStatus, DeployStepKey, EventKind, EventLevel, OutputStream, StepState } from "@/domain/model";
import { DELIVERY_REASONS, isProvider } from "@/domain/notifications";

import type { CodedActionRun, CodedEvent, MessageCode } from "../message-code";
import type { RuntimeView } from "@/features/activity/runtime-presentation";
import { reason, record, bool, id, date, optionalDate } from "./update-validation";
import { inventoryId } from "./inventory-mapping";
import { backupCodeKey } from "../backup-error";
import { VULNERABILITY_CODES } from "./vulnerability-decoders";

type Activity = components["schemas"]["ActivityActivity"];
type Detail = components["schemas"]["ActivityDetail"];
type Event = components["schemas"]["EventEvent"];

// Recreate run step names (backend recreate_journal.go) with catalogued EN/DE text, so the
// generic action dialog, toasts and history show the same labels as the recreate view.
const RECREATE_STEPS: Record<string, string> = { "Renaming container": "renaming", "Creating replacement": "creating", "Starting replacement": "starting", "Removing previous container": "removing", "Restoring previous container": "restoring" };
const STEP_KEYS = new Set<DeployStepKey>(["validate", "pull", "build", "create", "stop", "start", "health", "scan", "collect", "save", "wait", "connect"]);
const STREAMS = new Set<OutputStream>(["cmd", "stdout", "stderr", "note"]);
const KINDS: Record<string, ActionKind> = {
  image_pull: "pull", image_update_check: "check-updates", image_patch: "update",
  project_update: "update", project_restore: "restore",
  project_deploy: "deploy", project_redeploy: "redeploy", project_down: "stop",
  project_restart: "restart", container_start: "start", container_stop: "stop",
  container_restart: "restart", container_redeploy: "redeploy", container_edit: "recreate",
  vulnerability_scan: "scan", system_prune: "prune", auto_update: "update",
  container_auto_heal: "auto-heal",
  load_sustained: "load-alert",
};

// Hub-owned scheduler jobs carry an English job name and raw scheduler states as
// step names; their runs show catalogued labels instead.
const JOB_STEPS = new Set(["queued", "running", "waiting", "retrying", "succeeded", "failed", "partial", "needs_attention", "canceled", "skipped"]);
function jobKind(run: Activity): ActionKind | undefined {
  if (run.type !== "job_run" || run.resourceType !== "job") return undefined;
  if (run.resourceId === "update-check") return "check-updates";
  return run.resourceId?.startsWith("update-window:") ? "update" : undefined;
}

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
  // Auto Heal, notification delivery, load and condition codes live under runMessages.codes, keyed by their wire path.
  if (code && /^(autoheal|notification|load|container\.condition)\./.test(code)) return { code: `runMessages.codes.${code}`, params: {}, sourceText };
  return code ? { code, params: code === "activity.message" ? { message: sourceText } : {}, sourceText } : { code: "runMessages.value", params: { value: sourceText } };
}

const CONDITION_CODES = ["down", "unhealthy", "restart-loop", "running-again"] as const;
const count = (value: unknown) => (typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : 0);

/** A runtime condition event names its container, the exit code and the restarts of the sampled window. */
function conditionTitle(event: Event, meta: Record<string, unknown> | undefined): MessageCode | undefined {
  const kind = event.messageCode?.startsWith("container.condition.") ? event.messageCode.slice("container.condition.".length) : undefined;
  if (!kind || !(CONDITION_CODES as readonly string[]).includes(kind)) return undefined;
  const name = typeof meta?.containerName === "string" && meta.containerName ? meta.containerName : event.resourceName ?? "";
  return { code: `runMessages.codes.container.condition.${kind}`, params: { name, exitCode: count(meta?.exitCode), restarts: count(meta?.restarts) }, sourceText: event.title };
}

// Hub event codes with a catalogued title; the type keeps this list complete against the event codes of the contract.
type HubEventCode = Exclude<Extract<Event["messageCode"], `${"container" | "image" | "project" | "volume" | "network" | "system" | "user" | "environment" | "webhook"}.${string}`>, `container.condition.${string}` | "container.recreate-recovered" | "container.recreate-ambiguous" | "container.origin-locked">;
export const HUB_EVENT_CODES: readonly string[] = Object.keys({
  "container.start": true,
  "container.stop": true,
  "container.restart": true,
  "container.delete": true,
  "container.create": true,
  "container.scan": true,
  "container.update": true,
  "container.deploy": true,
  "container.kill": true,
  "container.pause": true,
  "container.unpause": true,
  "container.die": true,
  "container.oom": true,
  "container.rename": true,
  "container.unhealthy": true,
  "container.error": true,
  "image.pull": true,
  "image.load": true,
  "image.import": true,
  "image.tag": true,
  "image.untag": true,
  "image.commit": true,
  "image.delete": true,
  "image.prune": true,
  "image.scan": true,
  "image.vulnerability_scan": true,
  "image.error": true,
  "project.deploy": true,
  "project.delete": true,
  "project.start": true,
  "project.stop": true,
  "project.create": true,
  "project.update": true,
  "project.error": true,
  "volume.create": true,
  "volume.rename": true,
  "volume.delete": true,
  "volume.prune": true,
  "volume.error": true,
  "volume.file.create": true,
  "volume.file.delete": true,
  "volume.file.upload": true,
  "volume.file.update": true,
  "volume.workspace.update": true,
  "volume.backup.create": true,
  "volume.backup.delete": true,
  "volume.backup.restore": true,
  "volume.backup.restore_files": true,
  "volume.backup.download": true,
  "volume.backup.target.key_viewed": true,
  "network.create": true,
  "network.delete": true,
  "network.connect": true,
  "network.disconnect": true,
  "network.prune": true,
  "network.error": true,
  "system.prune": true,
  "system.auto_update": true,
  "system.upgrade": true,
  "user.login": true,
  "user.logout": true,
  "environment.create": true,
  "environment.connect": true,
  "environment.disconnect": true,
  "environment.update": true,
  "environment.delete": true,
  "environment.api_key.regenerated": true,
  "environment.mtls.ca_generated": true,
  "environment.mtls.cert_issued": true,
  "environment.mtls.enroll": true,
  "environment.mtls.download": true,
  "webhook.create": true,
  "webhook.update": true,
  "webhook.delete": true,
  "webhook.trigger": true,
} satisfies Record<HubEventCode, true>);
const EVENT_TITLES = new Set(HUB_EVENT_CODES);

/** Catalogue key of an event title: the wire code with underscores for dots, renamed where it reads like a credential. */
export const eventTitleKey = (code: string) => (code === "environment.api_key.regenerated" ? "environment_host_key_regenerated" : code.replaceAll(".", "_"));

// Bulk prunes record a marker instead of a resource: no resource id, a prune code or action, and one of these names.
const PRUNE_MARKERS = new Set(["bulk_prune", "prune"]);
// Phases of an automatic update run, which share one event code; each names its own target from the metadata.
const AUTO_UPDATE_PHASES = new Map<string, { key: string; name: string[] }>([
  ["start", { key: "start", name: [] }], ["complete", { key: "complete", name: [] }],
  ["image", { key: "image_pull", name: ["imageNew", "imageOld"] }], ["image_pull", { key: "image_pull", name: ["imageNew", "imageOld"] }],
  ["image_prune", { key: "image_prune", name: ["imageId"] }],
  ["container", { key: "container", name: ["resourceName", "container", "containerId"] }], ["project", { key: "project", name: ["projectName", "projectId"] }],
]);
/** Variant keys of codes whose producers tell phase or outcome apart by metadata or severity. */
export const EVENT_TITLE_VARIANTS = [...new Set([...AUTO_UPDATE_PHASES.values()].map(({ key }) => `system_auto_update_${key}`))].concat("system_upgrade_failed", "system_upgrade_partial", "webhook_trigger_failed");

/** Catalogue key and named target of an event, keeping the phase and outcome its producer recorded. */
function eventLabel(event: Event): { key: string; name: string } {
  const meta = (event.metadata ?? {}) as Record<string, unknown>;
  const text = (value: unknown) => (typeof value === "string" ? value : "");
  if (event.messageCode === "system.auto_update") {
    const phase = AUTO_UPDATE_PHASES.get(text(meta.phase));
    return phase ? { key: `system_auto_update_${phase.key}`, name: phase.name.map((field) => text(meta[field])).find(Boolean) ?? "" } : { key: "system_auto_update", name: "" };
  }
  const key = eventTitleKey(event.messageCode);
  const outcome = event.severity === "error" && (event.messageCode === "system.upgrade" || event.messageCode === "webhook.trigger") ? "_failed"
    : event.severity === "warning" && event.messageCode === "system.upgrade" ? "_partial" : "";
  const name = event.resourceName || (event.messageCode.startsWith("user.") ? event.username : "") || "";
  const marker = !event.resourceId && PRUNE_MARKERS.has(name) && (event.messageCode.endsWith(".prune") || meta.action === "prune");
  return { key: key + outcome, name: marker ? "" : name };
}

/** A hub event title from the catalogue, naming its resource or user as data. */
function hubEventTitle(event: Event): MessageCode | undefined {
  if (!EVENT_TITLES.has(event.messageCode)) return undefined;
  const { key, name } = eventLabel(event);
  const title = { code: `runMessages.events.labels.${key}`, params: {} };
  return name ? { code: "runMessages.events.named", params: { title, name }, sourceText: event.title } : { ...title, sourceText: event.title };
}

/** A delivery failure names its channel and typed reason; sender diagnostics never reach the title. */
function eventTitle(event: Event): MessageCode {
  const meta = event.metadata as Record<string, unknown> | undefined;
  const condition = conditionTitle(event, meta);
  if (condition) return condition;
  const provider = meta?.provider;
  // A sent notification names its channel; the hub's English title is only the fallback.
  if (event.messageCode === "notification.send") {
    const channel = isProvider(provider) ? { code: `settings.notifications.kinds.${provider}`, params: {} } : String(provider ?? event.resourceName ?? "");
    return { code: "runMessages.codes.notification.send", params: { provider: channel }, sourceText: event.title };
  }
  if (event.messageCode !== "notification.delivery-failed" || !isProvider(provider)) return hubEventTitle(event) ?? message(event.messageCode, event.title);
  const via = { code: `settings.notifications.kinds.${provider}`, params: {} };
  const reason = meta?.reason;
  return typeof reason === "string" && (DELIVERY_REASONS as readonly string[]).includes(reason)
    ? { code: "runMessages.codes.notification.delivery-failed-reason", params: { provider: via, reason: { code: `settings.codes.${reason}`, params: {} } }, sourceText: event.title }
    : { code: "runMessages.codes.notification.delivery-failed-via", params: { provider: via }, sourceText: event.title };
}

function knownBackupError(error: string): MessageCode | undefined {
  const key = backupCodeKey(error);
  return key ? { code: key, params: {} } : undefined;
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
  const kind: ActionKind = backup ? action.startsWith("restore_") ? "volume-restore" : "backup" : KINDS[run.type] ?? jobKind(run) ?? "activity";
  const job = run.type === "job_run";
  // The hub's fallback job summary is "<job name>: <scheduler state>"; a real outcome message stays.
  const jobSummary = (text: string | undefined): MessageCode | undefined => {
    const state = run.messageCode?.slice("job.".length) ?? "";
    return job && run.messageCode?.startsWith("job.") && JOB_STEPS.has(state) && text === `${run.resourceName ?? run.resourceId}: ${state}` ? { code: `activity.jobStep.${state}`, params: {} } : undefined;
  };
  const lifecycle = run.imageLifecycle;
  const m10 = run.type === "project_update" || run.type === "project_restore" || !!lifecycle;
  if (m10) { date(run.startedAt); optionalDate(run.endedAt); }
  const coded = (code: string | undefined, source: string, field: "progress" | "error" = "progress"): MessageCode => {
    if (backup) {
      const key = backupCodeKey(code) ?? backupCodeKey(source);
      if (key) return { code: key, params: {} };
      return { code: field === "error" ? "common.commandErrors.failed" : `activity.runKind.${kind}`, params: {} };
    }
    const operationReason = operationReasonKey(code ?? source);
    if (operationReason) return { code: `containers.operationReasons.${operationReason}`, params: {} };
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
    ...(lifecycle ? { baselineCode: reason(lifecycle.baselineCode), lifecycleCode: reason(lifecycle.code), effectVerified: lifecycle.verified, restorePointId: lifecycle.restorePointId, pointCode: reason(lifecycle.pointCode) } : {}),
    kind, sourceOperation: run.type,
    title: { code: `activity.runKind.${kind}`, params: {} },
    target: jobKind(run) ? { code: `activity.runKind.${kind}`, params: {} } as MessageCode : { code: "runMessages.value", params: { value: target } },
    hostId: run.environmentId,
    ...((run.resourceType === "project" || run.resourceType === "stack") && run.resourceId ? { stackId: inventoryId(run.environmentId, run.resourceId) } : {}),
    ...(run.resourceType === "container" && run.resourceId ? { containerId: inventoryId(run.environmentId, run.resourceId) } : {}),
    services: run.affectedServices ?? [],
    status: mapStatus(run.status),
    ...(run.outcomeState !== undefined ? { outcomeState: run.outcomeState, cancelRequested: run.cancelRequested } : {}),
    // A settled Auto Heal restart is one complete container action, not zero progress.
    progress: kind === "auto-heal" && run.status !== "running" && run.status !== "queued" ? 100 : run.progress ?? 0,
    startedAt: run.startedAt,
    finishedAt: run.endedAt,
    // Known backup codes are translated even when older job rows carry no backup context.
    error: run.error
      ? jobSummary(run.error) ?? (backup ? coded(run.error, "", "error") : knownBackupError(run.error) ?? vulnerabilityError(run.error))
        ?? coded(
          lifecycle?.code ?? (run.messageCode?.startsWith("runtime.") || run.messageCode?.startsWith("container.recreate-") ? run.messageCode : undefined),
          run.error,
          "error",
        )
      : undefined,
    steps: (detail.stepTrace ?? []).map((step) => ({
      key: stepKey(step.name), state: stepState(step.status),
      label: Object.hasOwn(RECREATE_STEPS, step.name) ? { code: `containers.recreate.steps.${RECREATE_STEPS[step.name]}`, params: {}, sourceText: step.name }
        : job && JOB_STEPS.has(step.name) ? { code: `activity.jobStep.${step.name}`, params: {} } : coded(step.code, step.name),
      startedAt: step.startedAt, finishedAt: step.endedAt,
    })),
    log: detail.messages?.length ? detail.messages.map((item) => coded(item.code, item.message)) : run.latestMessage ? [jobSummary(run.latestMessage) ?? coded(run.messageCode, run.latestMessage)] : [],
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
    title: eventTitle(event),
    hostId: event.environmentId,
    ...(event.messageCode?.startsWith("container.condition.") ? { condition: true } : {}),
    ...(event.resourceType === "project" && event.resourceId && event.environmentId ? { stackId: inventoryId(event.environmentId, event.resourceId) } : {}),
  };
}
