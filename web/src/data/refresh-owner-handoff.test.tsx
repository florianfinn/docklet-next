// Source: docklet-next/issues/391
// Revision: f2a8411892f4f9c2c33bcb98c56e28b800aba43f
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review R1G-1..R1G-3 at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { QueryClientProvider, isCancelledError, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useImages } from "./queries";
import { useBackupOverviews } from "./backup-queries";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";
import { backupFixture, overview } from "./http/backups.test-support";
import { createAppQueryClient } from "./app-query-client";
import { refresh } from "./refresh-owner";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };

it.each(["images", "backup-overviews"] as const)("R1G-1 a superseded first load of %s publishes no partial rows", async (kind) => {
  vi.useFakeTimers();
  const f = recoveryFixture();
  await f.adapter.getCurrentUser();
  const suffix = kind === "images" ? "images" : "volumes/backups/overview";
  for (const host of ["first", "second"]) f.state.hold.add(`/api/environments/${host}/${suffix}`);
  const call = vi.spyOn(f.adapter, kind === "images" ? "listImages" : "listBackupOverviews");
  let client!: QueryClient;
  function Images() { useImages(); return null; }
  function Backups() { useBackupOverviews(); return null; }
  function Client() { client = useQueryClient(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Client />{kind === "images" ? <Images /> : <Backups />}</IntlProvider></AdapterProvider>);
  await tick(100);
  expect(f.pending.filter((p) => p.path.endsWith(suffix))).toHaveLength(2);
  if (kind === "images") { f.state.edge = "offline"; f.send(); } else f.source.onopen?.({} as Event);
  await tick();
  const oldPartials: number[] = [];
  const off = client.getQueryCache().subscribe((event) => {
    const rows = client.getQueryData<unknown[]>([kind]);
    if (event.type === "updated" && event.query.queryKey[0] === kind && event.action.type === "success" && call.mock.calls.length === 1 && rows?.length) oldPartials.push(rows.length);
  });
  f.release(); await tick(100); off();
  expect([call.mock.calls.length > 1, oldPartials]).toEqual([true, []]);
});

it("R1G-2 a backup completion ends a completed shared overview before the next background read", async () => {
  vi.useFakeTimers();
  const f = recoveryFixture(); await f.adapter.getCurrentUser();
  const off = f.adapter.subscribeChanges(() => {});
  await f.adapter.getBackupOverview("first", "background");
  const reads = () => f.requests.filter((p) => p.path === "/api/environments/first/volumes/backups/overview").length;
  const policy = overview.volumes![0]!.policy!, oldNextRun = policy.nextRunAt, nextRun = "2026-12-01T01:00:00Z";
  try {
    policy.nextRunAt = nextRun;
    f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-11-11T12:00:01Z", activity: { type: "activity", environmentId: "first", activity: { id: "example-backup", type: "resource_action", resourceType: "volume", resourceId: "data", environmentId: "first", createdAt: "2026-11-11T12:00:00Z", endedAt: "2026-11-11T12:00:01Z", metadata: { action: "backup_volume" }, status: "success" } } }), lastEventId: "" } as MessageEvent);
    const after = await f.adapter.getBackupOverview("first", "background");
    expect([reads(), after.volumes[0]?.backup?.nextRunAt]).toEqual([2, nextRun]);
  } finally { policy.nextRunAt = oldNextRun; off(); }
});

it("R1G-3 a change after the last check but before Query publishes is refreshed once more", async () => {
  const client = createAppQueryClient(); let finish!: (value: string) => void; let calls = 0;
  const old = new Promise<string>((resolve) => { finish = resolve; });
  const queryFn = () => ++calls === 1 ? old : Promise.resolve("fresh");
  let fetchingAtChange: string | undefined;
  function Reader() { useQuery({ queryKey: ["handoff"], queryFn }); return null; }
  render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>);
  await vi.waitFor(() => expect(calls).toBe(1));
  finish("old");
  queueMicrotask(() => { fetchingAtChange = client.getQueryState(["handoff"])?.fetchStatus; refresh(client, [["handoff"]]); });
  await vi.waitFor(() => expect(client.getQueryData(["handoff"])).toBe("fresh"));
  expect([fetchingAtChange, calls, client.getQueryState(["handoff"])?.fetchStatus]).toEqual(["fetching", 2, "idle"]);
});

it("F-7 an earlier identity's in-flight shared overview rejects; the new identity reads again", async () => {
  const f = backupFixture(); let calls = 0, entered!: () => void, release!: () => void;
  const reached = new Promise<void>((resolve) => { entered = resolve; }), held = new Promise<void>((resolve) => { release = resolve; });
  f.intercept(async (url) => { if (url.pathname.endsWith("/overview") && ++calls === 1) { entered(); await held; } return undefined; });
  const old = f.api.getBackupOverview("first", "background").catch((error: unknown) => error);
  await reached; f.rotate();
  const reading = f.api.getBackupOverview("first", "background").then((row) => row.hostId, (error: unknown) => error);
  release();
  const current = await reading;
  expect([isCancelledError(await old), current, calls]).toEqual([true, "first", 2]);
});
