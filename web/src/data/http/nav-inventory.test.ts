// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original late-response, cold failure and identity reset shell counter coverage.

import { expect, it, vi } from "vitest";
import { isCancelledError } from "@tanstack/react-query";
import type { Host } from "@/domain/model";
import type { NavInventory } from "../nav-inventory";
import { createNavInventory, createNavInventoryState } from "./nav-inventory";
import { deferred } from "./activity-reader.test-support";
import { navFixture } from "@/app/shell/nav-counts.test-support";

it.each(["identity", "registration"])("row 9: discards a late host response after %s replacement without changing the badge", async (kind) => {
  const state = createNavInventoryState(); state.observe([{ id: "first", registrationRevision: 1 }]);
  let generation = 0, held = true, badge = 0;
  const late = deferred<Response>(), sent = deferred<void>();
  const fetcher: typeof fetch = async (input) => {
    if (String(input).includes("/containers")) return Response.json({ success: true, data: [], pagination: { totalItems: 0 } });
    if (held) { sent.resolve(); return late.promise; }
    return Response.json({ success: true, data: { total: 7 } });
  };
  const make = () => createNavInventory("/api", fetcher, async () => [{ id: "first", status: "online" } as Host], async () => {}, () => {}, () => generation, state.totals, state.registration);
  const publish = (rows: NavInventory[]) => { badge = rows.reduce((n, h) => n + (h.volumeTotal ?? 0), 0); };
  const pending = make().listNavInventory(publish).catch((error: unknown) => error);
  await sent.promise;
  if (kind === "identity") { generation++; state.reset(); }
  else state.observe([{ id: "first", registrationRevision: 2 }]);
  held = false;
  publish(await make().listNavInventory());
  expect(badge).toBe(7);
  late.resolve(Response.json({ success: true, data: { total: 99 } }));
  const outcome = await pending;
  expect(badge).toBe(7);
  expect(state.totals.get("first")).toBe(7);
  expect(isCancelledError(outcome)).toBe(true);
});

it.each(["count", "inventory"])("a cold %s failure does not invent a combined host volume total", async (leg) => {
  const f = navFixture();
  (leg === "count" ? f.state.countFailures : f.state.inventoryFailures).add("second");
  const batch = (await f.adapter.listNavInventory()).find((h) => h.hostId === "second")!;
  expect(batch.volumeTotal).toBeUndefined();
  expect(batch.inventoryUnavailable).toBe(leg === "inventory");
});

it("logout discards retained shell totals before the next authenticated identity reads", async () => {
  const f = navFixture();
  expect((await f.adapter.listNavInventory()).find((h) => h.hostId === "second")?.volumeTotal).toBe(4);
  await f.adapter.logout();
  await expect(f.adapter.listNavInventory()).rejects.toSatisfy(isCancelledError);
  await f.adapter.login("operator", "example");
  f.state.countFailures.add("second");
  const next = await f.adapter.listNavInventory();
  expect(next.find((h) => h.hostId === "second")?.volumeTotal).toBeUndefined();
});

it("host re-registration drops retained totals and an already dispatched old host answer", async () => {
  const f = navFixture();
  const off = f.adapter.subscribeChanges(() => {});
  await f.adapter.listNavInventory();
  const late = deferred<Response>(); f.state.delayHost = "second"; f.state.delay = late;
  const old = f.adapter.listNavInventory().catch((error: unknown) => error);
  await vi.waitFor(() => expect(f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/second/volumes/_collection/counts"))).toHaveLength(2));
  f.inventory.state.registrationRevision = 2;
  f.source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: "2026-11-14T12:00:00Z", environment: { environments: ["first", "second"].map((id) => ({ id, registrationRevision: 2, apiUrl: "http://agent.example.com", isEdge: false, status: "online", enabled: true })) } }) } as MessageEvent);
  late.resolve(Response.json({ success: true, data: { total: 99 } }));
  expect(isCancelledError(await old)).toBe(true);
  f.state.delayHost = ""; f.state.countFailures.add("second");
  const next = await f.adapter.listNavInventory();
  expect(next.find((h) => h.hostId === "second")?.volumeTotal).toBeUndefined();
  off();
});
