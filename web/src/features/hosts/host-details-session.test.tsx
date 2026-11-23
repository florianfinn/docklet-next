// Source: docklet-next/issues/579
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original host detail lifecycle proofs through the HTTP adapter for M37 #579.

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { AuthGate } from "@/app/AuthGate";
import { setAppLocale } from "@/app/i18n/locale-store";
import { deferred } from "@/data/http/activity-reader.test-support";
import { envelope, response } from "@/data/http/runtime-commands.test-support";
import { sessionFixture } from "@/data/http/session-lifecycle.test-support";
import { hostDetailsEpoch } from "@/data/host-details-source";
import { queryKeys } from "@/data/query-keys";
import { renderRoutes } from "@/test/render";

import HostsPage from "./HostsPage";

afterEach(() => setAppLocale("de"));

type Row = Record<string, unknown>;
const host = (patch: Row = {}): Row => ({ id: "atlas", name: "Example agent", apiUrl: "https://agent.example.com", enabled: true, status: "online", isEdge: false, registrationRevision: 1, ...patch });
const local: Row = { id: "0", name: "Local Docker", apiUrl: "https://hub.example.com", enabled: true, status: "online", isEdge: false, registrationRevision: 1 };

/** The real HTTP adapter with a mutable host list, held Docker info reads per host and the hub's current settings list. */
function hub(initial: Row[]) {
  const state = { rows: initial, user: "first-user", projectsDirectory: "/srv/projects-a" };
  const info: { host: string; done: ReturnType<typeof deferred<Response>> }[] = [];
  const f = sessionFixture((url) => {
    const path = url.pathname;
    // The host list carries the CSRF token pair, as the hub does; without it the client serialises reads behind its bootstrap.
    if (path.endsWith("/environments")) return new Response(JSON.stringify({ success: true, data: state.rows, pagination: { totalItems: state.rows.length } }), { headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path.endsWith("/auth/me")) return envelope({ id: state.user, username: state.user, requiresPasswordChange: false });
    const docker = /\/environments\/([^/]+)\/system\/docker\/info$/.exec(path);
    if (docker) { const done = deferred<Response>(); info.push({ host: decodeURIComponent(docker[1]!), done }); return done.promise; }
    if (path.endsWith("/environments/0/settings")) return response([{ key: "setupCompleted", value: "true" }, { key: "projectsDirectory", value: state.projectsDirectory }]);
  });
  return { f, state, info };
}

const answer = (os: string) => response({ OperatingSystem: os, ServerVersion: "28.5.1" });
const card = (id: string) => document.querySelector<HTMLElement>(`[data-host="${id}"]`);
const fact = (id: string, label: string) => [...(card(id)?.querySelectorAll("dt") ?? [])].find((dt) => dt.textContent === label)?.nextElementSibling?.textContent;

function render(h: ReturnType<typeof hub>) {
  let client!: QueryClient;
  function Page() { client = useQueryClient(); return <HostsPage />; }
  const view = renderRoutes([{ path: "/", element: <AuthGate />, children: [{ path: "hosts", element: <Page /> }] }], ["/hosts"], h.f.adapter);
  return { view, client: () => client };
}

it("a registration rotation in the host list never shows facts the previous registration answered", async () => {
  setAppLocale("en-US");
  const h = hub([host()]);
  const { view } = render(h);
  await waitFor(() => expect(h.info.filter((r) => r.host === "atlas").length).toBe(1));
  h.state.rows = [host({ registrationRevision: 2 })];
  // The hub's environments channel announces the change; the adapter reads the host list again.
  act(() => h.f.sources[0]!.environments());
  await waitFor(() => expect(h.info.filter((r) => r.host === "atlas").length).toBe(2));
  await act(async () => { h.info[0]!.done.resolve(answer("Previous registration OS")); });
  expect(document.body.textContent?.includes("Previous registration OS")).toBe(false);
  await act(async () => { h.info[1]!.done.resolve(answer("Current registration OS")); });
  await waitFor(() => expect(fact("atlas", "System")).toBe("Current registration OS"));
  expect(document.body.textContent?.includes("Previous registration OS")).toBe(false);
  view.unmount();
});

it("a held read of the previous user is never shown after the identity changes", async () => {
  setAppLocale("en-US");
  const h = hub([host()]);
  const { view, client } = render(h);
  await waitFor(() => expect(h.info.length).toBe(1));
  h.state.user = "second-user";
  await act(async () => { await client().refetchQueries({ queryKey: queryKeys.currentUser }); });
  await waitFor(() => expect(client().getQueryData(queryKeys.currentUser)).toMatchObject({ id: "second-user" }));
  await waitFor(() => expect(h.info.length).toBe(2));
  await act(async () => { h.info[0]!.done.resolve(answer("First user OS")); });
  expect(document.body.textContent?.includes("First user OS")).toBe(false);
  await act(async () => { h.info[1]!.done.resolve(answer("Second user OS")); });
  await waitFor(() => expect(fact("atlas", "System")).toBe("Second user OS"));
  expect(document.body.textContent?.includes("First user OS")).toBe(false);
  view.unmount();
});

it("a host removed while its read is held leaves no card, no fact and no further read", async () => {
  setAppLocale("en-US");
  const h = hub([local, host()]);
  const { view } = render(h);
  await waitFor(() => expect(h.info.some((r) => r.host === "atlas")).toBe(true));
  h.state.rows = [local];
  act(() => h.f.sources[0]!.environments());
  await waitFor(() => expect(card("atlas") === null).toBe(true));
  const reads = h.info.filter((r) => r.host === "atlas");
  await act(async () => { reads.forEach((r) => r.done.resolve(answer("Removed host OS"))); });
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(document.body.textContent?.includes("Removed host OS")).toBe(false);
  expect(h.info.filter((r) => r.host === "atlas").length).toBe(reads.length);
  view.unmount();
});

it("a stream reopen during the first held read repeats it, so the hub's superseded project directory is never shown", async () => {
  setAppLocale("en-US");
  const h = hub([local]);
  const { view } = render(h);
  await waitFor(() => expect(h.f.sources.length).toBe(1));
  // The card's first read has taken the settings ("/srv/projects-a") and waits for Docker info.
  await waitFor(() => expect(h.info.length).toBe(1));
  // The setting changes; the stream drops and the adapter opens a new one, which announces every topic again.
  h.state.projectsDirectory = "/srv/projects-b";
  act(() => h.f.sources[0]!.failClosed());
  await waitFor(() => expect(h.f.sources.length).toBe(2));
  act(() => h.f.sources[1]!.open());
  await act(async () => { h.info[0]!.done.resolve(answer("Example OS")); });
  await waitFor(() => expect(h.info.length).toBe(2));
  expect(document.body.textContent?.includes("/srv/projects-a")).toBe(false);
  await act(async () => { h.info[1]!.done.resolve(answer("Example OS")); });
  await waitFor(() => expect(fact("0", "Project directory")).toBe("/srv/projects-b"));
  expect(document.body.textContent?.includes("/srv/projects-a")).toBe(false);
  view.unmount();
});

it("a first read that fails after a host-list change starts a current read through the real adapter", async () => {
  setAppLocale("en-US");
  const h = hub([host()]);
  const original = h.f.adapter.getHostDetails!.bind(h.f.adapter);
  let started = 0;
  Object.assign(h.f.adapter, { getHostDetails: (id: string, signal?: AbortSignal) => { started++; return original(id, signal); } });
  const { view } = render(h);
  await waitFor(() => expect(h.info.length).toBe(1));
  const epoch = hostDetailsEpoch();
  act(() => h.f.sources[0]!.environments());
  await waitFor(() => expect(hostDetailsEpoch() > epoch).toBe(true));
  await act(async () => { h.info[0]!.done.resolve(response({ error: "old connection unavailable" }, 502)); });
  await act(async () => { await new Promise((r) => setTimeout(r, 100)); });
  // The failure belongs to the replaced state: the card starts a current read. The transport's host read admission
  // holds reads to a host that just failed, so this read stays in flight here; the deferred-adapter tests in
  // host-details.test.tsx cover its answer being shown.
  expect(started).toBe(2);
  expect(fact("atlas", "System")).toBe("Not reported by the agent");
  view.unmount();
});
