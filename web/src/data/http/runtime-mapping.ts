// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import type { components } from "@/api/generated/schema";
import type { CodedRuntimeActionRun } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import type { RuntimePresentation, RuntimeView } from "@/features/activity/runtime-presentation";
import { mapActivityDetail } from "./map-dto";

export type RuntimeSnapshot = components["schemas"]["ActivityRuntimeSnapshot"];
export type RuntimeDetail = components["schemas"]["ActivityRuntimeHubDetail"];
export type RuntimeRun = RuntimeView<CodedRuntimeActionRun>;
export const runtimeError = (code: string, status?: number) => new AdapterOutcomeError({ kind: "error", code, status });
export const object = (value: unknown): Record<string, unknown> => value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};

export function runtimeResponseError(body: unknown, status: number): AdapterOutcomeError {
  const envelope = object(body);
  const data = object(envelope.data);
  const code = data.code ?? object(envelope.error).code ?? envelope.code;
  if (status === 409 && code === "run.request-mismatch") return new AdapterOutcomeError({ kind: "conflict", code });
  if (status === 409 && (code === "run.busy" || code === "run.cancelling") && typeof data.activeRunId === "string" && data.activeRunId.trim()) {
    return new AdapterOutcomeError({ kind: "conflict", code, activeRunId: data.activeRunId });
  }
  // The frozen conflict port requires an ID. Missing authorized IDs stay coded errors.
  return runtimeError(typeof code === "string" ? code : "http.error", status);
}

export function mapRuntime(snapshot: unknown, hostId: string, id?: string, detail?: RuntimeDetail, rights?: RuntimePresentation): RuntimeRun {
  const value = object(snapshot);
  if (typeof value.id !== "string" || !value.id.trim() || (id !== undefined && value.id !== id) || value.environmentId !== hostId ||
      (value.outcomeState !== "known" && value.outcomeState !== "unknown") || typeof value.cancelRequested !== "boolean") throw runtimeError("runtime.invalid-response");
  try {
    const run = mapActivityDetail(detail ?? { activity: snapshot as RuntimeSnapshot, output: [], messages: [], stepTrace: [], outputTruncated: false });
    const recovery = detail?.recovery;
    if (detail && (!recovery || typeof recovery.executorState !== "string" || typeof recovery.releaseAcknowledged !== "boolean" ||
      (recovery.unknownReason !== undefined && typeof recovery.unknownReason !== "string"))) throw runtimeError("runtime.invalid-response");
    const result = object(value.result);
    if (value.result !== undefined && !["oldId", "newId", "oldName", "newName"].every((key) => typeof result[key] === "string" && result[key])) throw runtimeError("runtime.invalid-response");
    return { ...run, ...(value.result ? { recreateResult: value.result as NonNullable<RuntimeRun["recreateResult"]> } : {}), outcomeState: value.outcomeState as "known" | "unknown", cancelRequested: value.cancelRequested,
      runtime: { ...rights, sourceObserved: false, resultCode: typeof value.messageCode === "string" ? value.messageCode : undefined,
        unknownReason: recovery?.unknownReason, released: recovery?.disposition === "released", releaseReason: recovery?.dispositionReason,
        lateResult: recovery?.disposition ? recovery.executorResultCode : undefined } };
  } catch { throw runtimeError("runtime.invalid-response"); }
}
