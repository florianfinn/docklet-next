// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original lifecycle proofs for the Dashboard event owner: maintenance freshness and identity changes while gated (R1D-5, R1D-6).

import { isCancelledError } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { registerReadConsumer } from "../read-consumers";
import { at, deferred } from "./activity-reader.test-support";
import { createDashboardEvents } from "./dashboard-events";
import { response } from "./runtime-commands.test-support";
import { sessionFixture } from "./session-lifecycle.test-support";

const flush = async () => { for (let i = 0; i < 80; i++) await Promise.resolve(); };
const row = (id: string) => ({ id, type: "project.deploy", messageCode: "project.deploy", severity: "success", timestamp: at, createdAt: at, title: "Example" });
const frameEvent = () => ({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "changed", timestamp: at } }), lastEventId: "" }) as MessageEvent;
afterEach(() => { vi.useRealTimers(); });

/** A hub whose event window answer can be held after it took its snapshot, plus a maintenance status. */
async function hub() {
  let rows = [row("e1")], hold: Promise<void> | undefined, entered: ReturnType<typeof deferred<void>> | undefined, active = false;
  const f = sessionFixture((url) => {
    if (url.pathname === "/api/backups/maintenance") return Response.json({ active, operation: "restore" });
    if (url.pathname === "/api/events" && url.searchParams.has("since")) {
      const snapshot = [...rows], gate = hold; entered?.resolve();
      return (async () => { if (gate) await gate; return response({ success: true, data: snapshot, pagination: { totalItems: snapshot.length } }); })();
    }
    return undefined;
  });
  await f.adapter.getCurrentUser();
  const refetched: Array<ReturnType<typeof f.adapter.listDashboardEvents>> = [];
  const off = registerReadConsumer(f.adapter.subscribeChanges, "dashboardEvents", () => true);
  const stop = f.adapter.subscribeChanges((topics) => { if (topics.includes("dashboardEvents")) refetched.push(f.adapter.listDashboardEvents()); });
  f.sources[0]!.open(); f.sources[0]!.onmessage?.(frameEvent());
  const holdNext = () => {
    const gate = deferred<void>(); hold = gate.promise; entered = deferred<void>();
    return { entered: entered.promise, release: () => { hold = undefined; entered = undefined; gate.resolve(); } };
  };
  const maintenance = (value: boolean) => { active = value; return f.adapter.getHubBackupMaintenance(); };
  return { ...f, refetched, holdNext, maintenance, setRows: (next: string[]) => { rows = next.map(row); }, stop: () => { stop(); off(); } };
}

it("R1D-5 the refresh after hub maintenance does not join a snapshot taken before the restore", async () => {
  vi.useFakeTimers();
  const h = await hub();
  await h.adapter.listDashboardEvents();
  const held = h.holdNext();
  const old = h.adapter.listDashboardEvents();
  await held.entered;
  await h.maintenance(true); h.setRows(["e2", "e1"]); await h.maintenance(false); await flush();
  expect(h.refetched.length).toBe(1);
  held.release(); await old;
  const refreshed = await h.refetched[0]!;
  h.stop();
  expect(refreshed.events.map((e) => e.id)).toEqual(["e2", "e1"]);
});

it("R1D-6 a load waiting for the initial frame is cancelled by an identity reset and starts no read", async () => {
  vi.useFakeTimers();
  let generation = 0;
  const reads: number[] = [];
  const owner = createDashboardEvents(async () => { reads.push(generation); return generation; }, () => generation, { hasConsumer: () => true, changed: () => {} });
  owner.streamStarted(true);
  const old = owner.load().then((value) => ({ value }), (error: unknown) => ({ cancelled: isCancelledError(error) }));
  generation++; owner.identityChanged(); owner.streamStarted(true);
  await flush();
  owner.frame();
  expect([await old, reads]).toEqual([{ cancelled: true }, []]);
});

it("R1D-6 through the HTTP adapter: a load gated across a same-user sign-in waits for the new connection's initial frame and reads once", async () => {
  vi.useFakeTimers();
  let reads = 0;
  const f = sessionFixture((url) => url.pathname === "/api/events" && url.searchParams.has("since") ? (reads++, response({ success: true, data: [row("new-identity")], pagination: { totalItems: 1 } })) : undefined);
  await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeChanges(() => {});
  // The adapter re-reads a renewed same-principal session once; that re-read belongs to the new connection's gate.
  const old = f.adapter.listDashboardEvents().then((value) => value.events.map((e) => e.id), (error: unknown) => ({ cancelled: isCancelledError(error) }));
  await flush();
  expect(reads).toBe(0);
  await f.adapter.login("operator", "example-password"); await flush();
  const beforeFrame = reads;
  f.sources.at(-1)!.onmessage?.(frameEvent());
  const result = await old;
  stop();
  expect([beforeFrame, reads, result]).toEqual([0, 1, ["new-identity"]]);
});
