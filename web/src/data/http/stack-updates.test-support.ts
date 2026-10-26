// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer-shaped HTTP fixtures and behavioural guards for issue #231.

import { vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { ActivitySource } from "./activity-reader.test-support";
import { snapshot, runtimeDetail, RUN_ID, envelope, response } from "./runtime-commands.test-support";
import { observation } from "./update-observation.test-support";
import type { UpdateBaseline, StackUpdatePolicy, UpdatePoint, UpdateSchedules, SystemHints } from "@/domain/stack-updates";
export const revision = "a".repeat(64), targetIdentity = "b".repeat(64);
export const base: UpdateBaseline = { targetIdentity, savedRevision: revision, observedRevision: revision, available: true, autoExcluded: false };
export const point: UpdatePoint = { id: "128c02a8-584c-44ac-9920-717ec18cfdfa", registrationId: "direct", targetId: "project", targetIdentity, sourceRunId: RUN_ID, createdAt: "2026-10-26T00:00:00Z", sourceState: "settled", available: true, orphaned: false, volumeDataIncluded: false, images: [{ service: "app", reference: "example:1.0.0", imageId: "sha256:example", present: true, running: true }] };
export const storedPolicy: StackUpdatePolicy = { mode: "notify", revision: 1, environmentId: "direct", registrationId: "direct", targetKind: "project", targetId: "project", targetIdentity, jobId: "image-polling", stale: true, checkedAt: point.createdAt, applyCheckedAt: point.createdAt, applyCode: "update.config-drift", candidates: [{ service: "app", imageRef: "example:1.0.0", version: "1.1.0", digest: "sha256:example", platform: "linux/amd64", entryId: "candidate", notificationState: "sending" }] };
export const schedule: UpdateSchedules = { revision: 1, environmentId: "direct", registrationId: "direct", pollingInterval: "0 0 * * * *", autoUpdateInterval: "0 0 0 * * *", pollingEnabled: true, autoUpdateEnabled: false, masterPollingEnabled: true, masterAutoUpdateEnabled: false, timeZone: "Europe/Berlin", jobs: [{ id: "image-polling", kind: "update-check", schedule: "0 0 * * * *", enabled: false, code: "update.schedule-owner-unset", currentRun: { id: RUN_ID, status: "running" } }, { id: "auto-update", kind: "auto-update", schedule: "0 0 0 * * *", enabled: false }] };
export const systemHints: SystemHints = { hubVersion: "2.0.0", images: [{ kind: "agent", reference: "example:1.9.0", currentVersion: "1.9.0", candidateVersion: "2.0.0", state: "available", requiresHubUpdate: true, manualOnly: true }, { kind: "tools", reference: "example:4.0.0", currentVersion: "4.0.0", state: "pinned", requiresHubUpdate: false, manualOnly: true }] };
export const updateOperations = ["listHostRestorePoints", "getUpdateBaseline", "listRestorePoints", "getAutoUpdatePolicy", "setAutoUpdate", "setAutoUpdateApply", "listSchedule", "saveUpdateSchedule", "listSystemComponents", "checkUpdates", "installStackUpdates", "restoreRestorePoint", "listHosts", "listActions"];
export function updateFixture() {
  const state = { isEdge: false, denied: new Set<string>(), baseline: structuredClone(base), policy: structuredClone(storedPolicy), points: [structuredClone(point)], schedule: structuredClone(schedule), hints: structuredClone(systemHints), code: "", status: 409, malformed: false, hold: undefined as undefined | ((path: string, init: RequestInit) => Promise<Response> | undefined), hostFailure: "", user: "operator" };
  const calls: { path: string; init: RequestInit }[] = [];
  const run = snapshot("direct", { type: "project_update", resourceType: "stack", resourceId: "project", outcomeState: "known" });
  const source = new ActivitySource();
  const fetcher = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname; calls.push({ path, init });
    if (path.endsWith("/capabilities")) return envelope(updateOperations.map((operationId) => ({ operationId, available: !state.denied.has(operationId), ...(state.denied.has(operationId) ? { disabledReasonCode: "capability.unavailable" } : {}), resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (path.endsWith("/auth/me")) return envelope({ id: state.user, username: state.user, requiresPasswordChange: false, isGlobalAdmin: true, permissionsByEnv: { global: ["activities:read"] }, locale: "de" });
    if (path.endsWith("/auth/mode")) return envelope({ mode: "local" });
    if (path.includes("/auth/")) return envelope({});
    if (path.endsWith("/settings")) return envelope({ effectiveTimeZone: "UTC", settings: [] });
    if (path.endsWith("/environments")) return response({ success: true, data: ["direct", "other"].map((id) => ({ id, name: id, apiUrl: "https://agent.example.com", enabled: true, isEdge: state.isEdge, status: "online" })), pagination: { totalItems: 2 } });
    const held = state.hold?.(path, init); if (held) return held;
    if (path.includes(`/environments/${state.hostFailure}/`) && state.hostFailure) return response({ code: "agent.unavailable" }, 502);
    if (path.endsWith("/activities")) return response({ success: true, data: [], pagination: { totalItems: 0 } });
    if (path.includes("/runtime-runs/")) return envelope(runtimeDetail(run));
    if (path.endsWith("/baseline")) return envelope(state.malformed ? { ...state.baseline, available: "yes" } : state.baseline);
    if (path.endsWith("/restore-points")) return envelope(state.points);
    if (path.endsWith("/check")) return state.code ? response({ success: false, error: { code: state.code, message: "raw diagnostic" } }, state.status) : envelope({ status: "has_update", hasUpdate: true, imageCount: 1, checkedImageCount: 1, imagesWithUpdates: 1, imagesNotPulled: 0, errorCount: 0, serviceUpdates: { app: { imageRef: "example:1.0.0", updateInfo: observation("version") } } });
    if (path.endsWith("/restore")) run.type = "project_restore";
    if (path.endsWith("/update") || path.endsWith("/restore")) return state.code ? response({ success: false, error: { code: state.code, message: "raw diagnostic" } }, state.status) : envelope({ activityId: run.id, code: "run.started", activity: run }, 202);
    if (path.includes("/policies/")) { if (init.method === "PUT") { if (state.code) return response({ success: false, error: { code: state.code, message: "raw diagnostic" } }, state.status); const input = JSON.parse(String(init.body)); state.policy = { ...state.policy, mode: input.mode, revision: state.policy.revision + 1 }; } return envelope(state.policy); }
    if (path.endsWith("/schedules")) { const host = path.split("/")[3]!; if (init.method === "PUT") { if (state.code) return response({ success: false, error: { code: state.code, message: "raw diagnostic" } }, state.status); const input = JSON.parse(String(init.body)); state.schedule = { ...state.schedule, ...input, revision: state.schedule.revision + 1, jobs: state.schedule.jobs.map((j) => ({ ...j, schedule: j.kind === "update-check" ? input.pollingInterval : input.autoUpdateInterval, code: undefined })) }; } return envelope({ ...state.schedule, environmentId: host, registrationId: host }); }
    if (path.endsWith("/system-image-hints")) return envelope(state.hints);
    return response({}, 404);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source as unknown as EventSource });
  const writes = () => calls.filter((c) => c.init.method === "POST" || c.init.method === "PUT");
  return { state, adapter, calls, source, writes, fetcher, run };
}
