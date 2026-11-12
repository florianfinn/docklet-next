// Source: docklet-next/issues/389
// Revision: a331ec9df0c18670fd14d06d043cf58a4ef0fc80
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original red reproductions of M23 MNT1 and OFF1 at the HTTP adapter and app provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { isCancelledError, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { useEffect, type ReactNode } from "react";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "../AdapterProvider";
import { useActiveSecurity, useImages, useSchedule, useVolumes } from "../queries";
import { useBackupOverviews } from "../backup-queries";
import { QueryError } from "@/shared/components/page";
import { queryKeys } from "../query-keys";
import { identityLifetime } from "../identity-lifetime";
import { recoveryFixture } from "./recovery-diagnosis.test-support";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };
function Client({ ready }: { ready: (client: QueryClient) => void }) {
  const client = useQueryClient();
  useEffect(() => { ready(client); }, [client, ready]);
  return null;
}
async function mount(f: ReturnType<typeof recoveryFixture>, children: ReactNode) {
  await f.adapter.getCurrentUser();
  let client!: QueryClient;
  const ready = (value: QueryClient) => { client = value; };
  const view = render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Client ready={ready} />{children}</IntlProvider></AdapterProvider>);
  await tick(); f.source.onopen?.({} as Event); f.send(); await tick(100);
  return { client, view };
}
function Reads() { useImages(); useVolumes(); useActiveSecurity(); return null; }
function Schedule() {
  const query = useSchedule(["first", "second"]);
  return query.isError ? <QueryError error={query.error} compact onRetry={() => void query.refetch()} /> : <p>schedule {query.status}</p>;
}
function Overviews({ volumes = false }: { volumes?: boolean }) {
  const query = useBackupOverviews();
  return <>{volumes && <Volumes />}<p>backups {query.status}</p></>;
}
function Volumes() { useVolumes(); return null; }

it.each(["environment", "stream reopen"] as const)("MNT1 %s refresh after maintenance must not abort held-class reads", async (trigger) => {
  vi.useFakeTimers(); const f = recoveryFixture();
  const { client } = await mount(f, <Reads />);
  expect(client.getQueryState(queryKeys.activeSecurity)?.status).toBe("success");
  let resets = 0; const offIdentity = identityLifetime(f.adapter).listen(() => { resets++; });
  f.state.maintenance = true; await f.adapter.getHubBackupMaintenance();
  const paths = ["/api/environments/first/images", "/api/environments/first/volumes", "/api/environments/first/volumes/binds", "/api/environments/first/images/vulnerabilities/active-summaries"];
  paths.forEach((path) => f.state.hold.add(path));
  f.state.maintenance = false; await f.adapter.getHubBackupMaintenance(); await tick(100);
  expect(paths.every((path) => f.pending.some((p) => p.path === path))).toBe(true);
  expect(f.pending.some((p) => p.aborted)).toBe(false);
  if (trigger === "environment") { f.state.edge = "offline"; f.send(); }
  else f.source.onopen?.({} as Event);
  await tick(100);
  const aborted = f.pending.filter((p) => p.aborted).map((p) => p.path);
  f.release(); await tick(100); offIdentity();
  expect(resets).toBe(0);
  expect(aborted).toEqual([]);
});

it("MNT1 a schedule read fenced by an environment change must recover without QueryError", async () => {
  vi.useFakeTimers(); const f = recoveryFixture();
  const { client, view } = await mount(f, <Schedule />);
  expect(client.getQueryState(["schedule", ["first", "second"]])?.status).toBe("success");
  f.state.maintenance = true; await f.adapter.getHubBackupMaintenance();
  f.state.hold.add("/api/environments/first/volumes/backups/overview");
  f.state.maintenance = false; await f.adapter.getHubBackupMaintenance(); await tick(100);
  expect(f.pending.some((p) => p.path.endsWith("/overview"))).toBe(true);
  f.state.edge = "offline"; f.send(); await tick();
  f.release(); await tick(100);
  const state = client.getQueryState(["schedule", ["first", "second"]]);
  console.log("MNT1 schedule outcome", state?.status, state?.error?.constructor.name, isCancelledError(state?.error), view.container.textContent);
  expect(view.container.textContent?.includes("Data could not be loaded")).toBe(false);
  expect(state?.status).toBe("success");
});

it("OFF1 two recovery frames alone refresh each overview once", async () => {
  vi.useFakeTimers(); const f = recoveryFixture();
  const { client } = await mount(f, <Overviews />);
  f.state.edge = "offline"; f.send(); await tick(100);
  f.requests.length = 0;
  f.state.edge = "pending"; f.send(); await tick(137);
  f.state.edge = "online"; f.send(); await tick(1000);
  expect(client.getQueryState(["backup-overviews"])?.status).toBe("success");
  expect(f.requests.filter((p) => p.path === "/api/environments/second/volumes/backups/overview")).toHaveLength(1);
});

it("OFF1 the overview query and shell volume decoration must share one recovery overview", async () => {
  vi.useFakeTimers(); const f = recoveryFixture();
  const { client } = await mount(f, <Overviews volumes />);
  f.state.edge = "offline"; f.send(); await tick(100);
  f.state.edge = "pending"; f.send(); await tick(137);
  f.requests.length = 0;
  // The volume fan-out decorates only after its inventory prerequisites have settled.
  f.state.hold.add("/api/environments/second/volumes");
  f.state.edge = "online"; f.send(); await tick(312);
  const first = f.requests.filter((p) => p.path === "/api/environments/second/volumes/backups/overview");
  expect(first).toHaveLength(1);
  expect(client.getQueryState(["backup-overviews"])?.fetchStatus).toBe("idle");
  expect(client.getQueryState(queryKeys.volumes)?.fetchStatus).toBe("fetching");
  f.release(); await tick();
  expect(client.getQueryState(["backup-overviews"])?.status).toBe("success");
  expect(client.getQueryState(queryKeys.volumes)?.status).toBe("success");
  expect(f.requests.filter((p) => p.path === "/api/environments/second/volumes/backups/overview")).toHaveLength(1);
});


it("MNT1 a response started before an environment change never replaces current counts", async () => {
  vi.useFakeTimers(); const f = recoveryFixture();
  const { client } = await mount(f, <Reads />);
  const counts: number[] = [];
  const off = client.getQueryCache().subscribe(() => {
    const value = client.getQueryData<{ images: Array<{ counts: { critical: number } }> }>(queryKeys.activeSecurity);
    if (value?.images[0]) counts.push(value.images[0].counts.critical);
  });
  f.state.critical = 2;
  f.state.hold.add("/api/environments/first/images/vulnerabilities/active-summaries");
  void client.invalidateQueries({ queryKey: queryKeys.activeSecurity }); await tick();
  expect(f.pending.length).toBeGreaterThan(0);
  f.state.critical = 3; f.state.edge = "offline"; f.send(); await tick();
  counts.length = 0; f.release(); await tick(100); off();
  expect(counts.includes(2)).toBe(false);
  expect(client.getQueryData<{ images: Array<{ counts: { critical: number } }> }>(queryKeys.activeSecurity)?.images[0]?.counts.critical).toBe(3);
});
