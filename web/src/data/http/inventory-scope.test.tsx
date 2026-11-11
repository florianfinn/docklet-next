// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary scope, reason, retention and lifetime regression coverage. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { afterEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { isCancelledError } from "@tanstack/react-query";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { enStacks as en } from "@/features/stacks/messages/en";
import { deStacks as de } from "@/features/stacks/messages/de";
import { inventoryRecoveryInterval } from "../inventory-recovery";
import { resourceFixture } from "./resources.test-support";
import { createInventoryOwner } from "./inventory-owner";
import { AdapterOutcomeError } from "../capability";
import { isMaintenanceError } from "../maintenance-retry";
import type { Host } from "@/domain/model";

const flush = async () => { for (let i = 0; i < 100; i++) await Promise.resolve(); };
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });
function fixture() {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const state = { fail: "", entered: false, release: undefined as (() => void) | undefined };
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/environments" && state.fail === "held") { state.entered = true; await new Promise<void>((resolve) => { state.release = resolve; }); }
    if (path === "/api/environments" && state.fail === "server") return Response.json({ code: "agent.unavailable" }, { status: 502 });
    if (path === "/api/environments" && state.fail === "session") return Response.json({ code: "auth.unauthorized" }, { status: 401 });
    const answer = await original(input);
    if (path === "/api/capabilities" && state.fail === "denied") {
      const body = await answer.json(); for (const c of body.data) if (c.operationId === "listContainers") { c.available = false; c.disabledReasonCode = "capability.unavailable"; }
      return Response.json(body, { headers: answer.headers });
    }
    return answer;
  });
  const stop = f.adapter.subscribeChanges(() => {});
  const invalidate = () => f.source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: "2026-11-11T12:00:00Z" }), lastEventId: "" } as MessageEvent);
  return { ...f, scope: state, stop, invalidate };
}
function freeze<T>(value: T): T {
  if (value && typeof value === "object") { Object.freeze(value); Object.values(value).forEach(freeze); }
  return value;
}

it("L4-f timeout after success retains scope with a separate host budget, stale flag and recovery", async () => {
  vi.useFakeTimers(); const f = fixture();
  const first = freeze(await f.adapter.listContainers());
  f.scope.fail = "held"; f.invalidate();
  const refresh = f.adapter.listContainers().catch((error: unknown) => error); await flush(); expect(f.scope.entered).toBe(true);
  await vi.advanceTimersByTimeAsync(30_001);
  const next = await refresh;
  expect(Array.isArray(next)).toBe(true); if (!Array.isArray(next)) return;
  expect(next.map((r) => r.id)).toEqual(first.map((r) => r.id));
  expect(next[0]?.inventory).toMatchObject({ stale: true, scopeStale: true });
  expect(next[0]).not.toBe(first[0]); expect(first[0]?.inventory?.stale).toBe(false);
  expect(inventoryRecoveryInterval(next, [{ id: "first", status: "online" } as Host])).toBe(15_000);
  f.scope.fail = ""; f.scope.release?.(); await flush(); f.stop();
});

it("L4-f first load scope failure is an error, and denial after success never retains", async () => {
  const f = fixture(); f.scope.fail = "server";
  await expect(f.adapter.listContainers()).rejects.toBeDefined();
  f.scope.fail = ""; f.invalidate(); await f.adapter.listContainers();
  f.scope.fail = "denied"; f.invalidate();
  await expect(f.adapter.listContainers()).rejects.toMatchObject({ outcome: { kind: "not-available" } });
  f.stop();
});

it("L4-f session loss resets catalogue instead of retaining it", async () => {
  const f = fixture(); await f.adapter.listContainers();
  f.scope.fail = "session"; f.invalidate();
  await expect(f.adapter.listContainers()).rejects.toBeDefined();
  expect((await f.adapter.listHosts().catch(() => []))).toEqual([]);
  f.stop();
});

it("L4-f delayed old-principal catalogue is discarded and cannot seed the new principal", async () => {
  const f = fixture(); f.scope.fail = "held";
  const old = f.adapter.listContainers().catch((error: unknown) => error);
  await vi.waitFor(() => expect(f.scope.entered).toBe(true));
  // An unrelated non-owned 401 exercises the adapter's authoritative identity reset.
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => String(input).endsWith("/backups/storage") ? new Response(null, { status: 401 }) : original(input));
  await f.adapter.getHubBackupStorage().catch(() => undefined);
  f.scope.fail = ""; f.scope.release?.();
  expect(isCancelledError(await old)).toBe(true);
  f.stop();
});

it("L4-n frozen catalogue and observations are copied on stale scope and reset removes retention", async () => {
  let generation = 0, fail = false;
  const rows = freeze([{ id: "first", status: "online", inventory: { stale: false } }]) as unknown as Host[];
  const owner = createInventoryOwner(() => generation, async () => { if (fail) throw new AdapterOutcomeError({ kind: "error", code: "http.error" }); return rows; }, async () => {}, fetch, () => {});
  const api = owner.wrap((_observe, _fetch, hosts) => ({ async listContainers() { return freeze((await hosts()).map((h) => ({ hostId: h.id, inventory: h.inventory }))); } }));
  const first = freeze(await api.listContainers()); fail = true;
  let failure: unknown;
  const second = await api.listContainers().catch((error: unknown) => { failure = error; return []; });
  expect(failure).toBeUndefined();
  expect(second[0]?.inventory).toMatchObject({ stale: true, scopeStale: true });
  expect(second[0]?.inventory).not.toBe(first[0]?.inventory);
  expect(rows[0]?.inventory).toEqual({ stale: false });
  generation++; owner.reset();
  await expect(api.listContainers()).rejects.toBeDefined();
});

it.each([["en-US", en, "Inventory catalogue could not be refreshed"], ["de", de, "Inventarkatalog konnte nicht aktualisiert werden"]] as const)("scope notice uses the %s catalogue", (locale, messages, text) => {
  render(<IntlProvider locale={locale} messages={{ stacks: messages }}><InventoryNotice hosts={[{ id: "first", name: "first", status: "online" } as Host]} items={[{ hostId: "first", inventory: { stale: true, scopeStale: true } }]} /></IntlProvider>);
  expect(screen.getByRole("status").textContent?.includes(text)).toBe(true);
});

it("L4-f scope owner rejects an uncooperative late answer after principal replacement", async () => {
  let generation = 0, fail = false, release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  const owner = createInventoryOwner(() => generation, async () => {
    if (fail) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
    await gate; return [{ id: "old", status: "online" }] as Host[];
  }, async () => {}, fetch, () => {});
  const api = owner.wrap((_observe, _fetch, hosts) => ({ async listContainers() { return hosts(); } }));
  const old = api.listContainers().catch((error: unknown) => error); await flush();
  generation++; owner.reset(); release();
  expect(isCancelledError(await old)).toBe(true);
  fail = true; await expect(api.listContainers()).rejects.toBeDefined();
});

/** An owner whose capability stage fails with `failure` after one successful load. */
async function failingScope(failure: unknown) {
  let fail = false;
  const rows = [{ id: "first", status: "online", inventory: { stale: false } }] as unknown as Host[];
  const owner = createInventoryOwner(() => 0, async () => rows, async () => { if (fail) throw failure; }, fetch, () => {});
  const api = owner.wrap((_observe, _fetch, hosts) => ({ async listContainers() { return (await hosts()).map((h) => ({ hostId: h.id, inventory: h.inventory })); } }));
  await api.listContainers(); fail = true;
  return api.listContainers().then((value) => value, (error: unknown) => ({ rejected: error }));
}

it("R1E-1 hub maintenance after success rejects with the maintenance outcome instead of a stale scope", async () => {
  const outcome = await failingScope(new AdapterOutcomeError({ kind: "error", code: "backup.maintenance", status: 503 }));
  expect("rejected" in outcome && isMaintenanceError(outcome.rejected)).toBe(true);
});

it.each([
  ["capability 5xx", new AdapterOutcomeError({ kind: "not-available", operationId: "listContainers", resourceRef: { kind: "hub", id: "0" }, reasonCode: "capability.read-unavailable" })],
  ["capability network", new TypeError("fetch failed")],
])("R1E-2 a transient %s after success retains the catalogue as stale scope", async (_case, failure) => {
  const outcome = await failingScope(failure);
  expect(Array.isArray(outcome) && outcome[0]?.inventory?.scopeStale === true).toBe(true);
});

it("R1E-2 a capability denial after success still rejects", async () => {
  const outcome = await failingScope(new AdapterOutcomeError({ kind: "not-available", operationId: "listContainers", resourceRef: { kind: "hub", id: "0" }, reasonCode: "capability.unavailable" }));
  expect("rejected" in outcome).toBe(true);
});

it.each([["en-US", en, "first: Inventory catalogue could not be refreshed. No retained"], ["de", de, "first: Inventarkatalog konnte nicht aktualisiert werden. Noch keine"]] as const)("R1E-4 a stale catalogue of an available host names no host reason (%s)", (locale, messages, text) => {
  render(<IntlProvider locale={locale} messages={{ stacks: messages }}><InventoryNotice hosts={[{ id: "first", name: "first", status: "online" } as Host]} items={[{ hostId: "first", inventory: { stale: true, scopeStale: true } }]} /></IntlProvider>);
  const content = screen.getByRole("status").textContent ?? "";
  expect([content.includes(text), content.includes(messages.inventory.unavailable)]).toEqual([true, false]);
});
