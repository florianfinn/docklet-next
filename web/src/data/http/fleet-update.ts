// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import { CancelledError } from "@tanstack/react-query";
import { validFleetResponse, validFleetStatus, type FleetResponse, type FleetStatus } from "@/domain/fleet-update";
import { sameProductVersion } from "@/domain/self-update";
import { FLEET_CODE_KEYS } from "../wire-catalogue-self-update";
import { wireKey } from "../wire-catalogue";
import { AdapterOutcomeError, unavailable } from "../capability";
import type { CsrfClient } from "./csrf-client";
import type { HubUpdateSession } from "./hub-update-session";
import { object, runtimeError } from "./runtime-mapping";

interface Options {
  baseUrl: string; client: CsrfClient; generation: () => number;
  capability: (operation: string, ref: { kind: "hub"; id: string }) => Promise<{ available: boolean; disabledReasonCode?: string }>;
  hubSession: HubUpdateSession;
}
export function createFleetUpdates({ baseUrl, client, generation, capability, hubSession }: Options) {
  let pending: Promise<FleetResponse> | undefined, pendingOwner: number | undefined;
  const fence = (owner: number) => { if (owner !== generation()) throw new CancelledError(); };
  async function request(targetVersion?: string): Promise<unknown> {
    const owner = generation(), controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const timeout = new Promise<never>((_, reject) => { timer = setTimeout(() => { controller.abort(); reject(new TypeError("timeout")); }, 15_000); });
    try {
      return await Promise.race([(async () => {
        const response = await client.fetch(`${baseUrl}/system/upgrade/all`, { method: targetVersion === undefined ? "GET" : "POST", cache: "no-store", credentials: "same-origin", signal: controller.signal,
          headers: { Accept: "application/json", ...(targetVersion === undefined ? {} : { "Content-Type": "application/json" }) },
          ...(targetVersion === undefined ? {} : { body: JSON.stringify({ targetVersion }) }) });
        fence(owner);
        // A gateway reply is ambiguous; only a hub problem with a known code is a refusal.
        if ([502, 503, 504].includes(response.status)) {
          const problem = object(await response.json().catch(() => undefined)); fence(owner);
          throw runtimeError(wireKey(FLEET_CODE_KEYS, String(problem.code)) ? String(problem.code) : "http.error", response.status);
        }
        const body: unknown = await response.json(); fence(owner);
        if (!response.ok) { const problem = object(body), code = wireKey(FLEET_CODE_KEYS, String(problem.code)) ? problem.code : wireKey(FLEET_CODE_KEYS, String(problem.detail)) ? problem.detail : "http.error"; throw runtimeError(String(code), response.status); }
        return body;
      })(), timeout]);
    } finally { clearTimeout(timer); fence(owner); }
  }
  async function getFleetUpdate(): Promise<FleetStatus> {
    const row = await request() as FleetStatus;
    if (!validFleetStatus(row)) throw runtimeError("http.invalid-response");
    return row;
  }
  hubSession.readFleetHub(async () => (await getFleetUpdate()).hub);
  return {
    getFleetUpdate,
    updateEverything(targetVersion: string): Promise<FleetResponse> {
      if (pending && pendingOwner === generation()) return pending;
      const owner = generation();
      const task = (async () => {
        const cap = await capability("selfUpdate", { kind: "hub", id: "0" }); fence(owner);
        if (!cap.available) throw unavailable("selfUpdate", { kind: "hub", id: "0" }, cap.disabledReasonCode);
        if (!targetVersion.trim()) throw runtimeError("update.invalid-target");
        let row: FleetResponse;
        try { row = await request(targetVersion) as FleetResponse; }
        catch (error) {
          fence(owner);
          const ambiguous = error instanceof TypeError || error instanceof DOMException && error.name === "AbortError" || error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "http.error" && [502, 503, 504].includes(error.outcome.status ?? 0);
          if (!ambiguous) throw error;
          const status = await getFleetUpdate().catch(() => undefined); fence(owner);
          if (!status?.active || !sameProductVersion(status.targetVersion, targetVersion)) throw error;
          row = { fleetId: status.fleetId!, runId: status.runId!, hubPhase: status.phase === "hub" && status.hub?.state !== "skipped" ? "update" : "current", hubOpId: status.hub?.opId };
        }
        fence(owner);
        if (!validFleetResponse(row)) throw runtimeError("http.invalid-response");
        if (row.hubPhase === "update") await hubSession.start(targetVersion, async () => ({ accepted: true, opId: row.hubOpId!, phase: "reserved" }), { combined: true });
        fence(owner);
        return row;
      })();
      pending = task; pendingOwner = owner;
      void task.finally(() => { if (pending === task) pending = undefined; }).catch(() => {});
      return task;
    },
  };
}
