// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original host detail read and card proofs for M37 #579.

import { act, waitFor } from "@testing-library/react";
import { useSyncExternalStore } from "react";
import { afterEach, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { createHostDetails } from "@/data/http/host-details";
import type { CsrfClient } from "@/data/http/csrf-client";
import { mapHost } from "@/data/http/hosts";
import type { Host, HostDetails } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { HostCard } from "./HostCard";
import { FAILED_READ_RETRY_MS } from "./use-host-details";

afterEach(() => { setAppLocale("de"); vi.useRealTimers(); });

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

function client(routes: Record<string, () => Response>): CsrfClient & { calls: string[] } {
  const calls: string[] = [];
  return { calls, fetch: async (input: RequestInfo | URL) => { const url = String(input); calls.push(url); const route = Object.keys(routes).find((path) => url.endsWith(path)); if (!route) throw new Error(`unexpected ${url}`); return routes[route]!(); } } as unknown as CsrfClient & { calls: string[] };
}

it("decodes the flat Docker info body for Direct and Edge hosts and adds the hub project directory only locally", async () => {
  const info = { OperatingSystem: "Debian GNU/Linux 13 (trixie)", ServerVersion: "28.5.1", OSType: "linux", Version: "ignored", os: "linux" };
  const http = client({
    "/environments/direct/system/docker/info": () => json(info),
    "/environments/edge/system/docker/info": () => json({ ...info, ServerVersion: "27.4.0" }),
    "/environments/0/system/docker/info": () => json(info),
    "/environments/0/settings": () => json([{ key: "projectsDirectory", value: "/srv/projects" }, { key: "agentProjectsDirectory", value: "/opt/agents" }]),
  });
  const { getHostDetails } = createHostDetails("/api", http);
  expect(await getHostDetails!("direct")).toEqual({ os: "Debian GNU/Linux 13 (trixie)", dockerVersion: "28.5.1" });
  expect(await getHostDetails!("edge")).toEqual({ os: "Debian GNU/Linux 13 (trixie)", dockerVersion: "27.4.0" });
  expect(await getHostDetails!("0")).toEqual({ os: "Debian GNU/Linux 13 (trixie)", dockerVersion: "28.5.1", projectDir: "/srv/projects" });
  expect(http.calls.filter((url) => url.endsWith("/settings")).length).toBe(1);
});

it("keeps Docker facts when the hub settings cannot be read and rejects a failed Docker read", async () => {
  const http = client({ "/environments/0/system/docker/info": () => json({ OperatingSystem: "Alpine Linux v3.22", ServerVersion: "28.0.0" }), "/environments/0/settings": () => json({ error: "nope" }, 500), "/environments/down/system/docker/info": () => json({ error: "unreachable" }, 502) });
  const { getHostDetails } = createHostDetails("/api", http);
  expect(await getHostDetails!("0")).toEqual({ os: "Alpine Linux v3.22", dockerVersion: "28.0.0", projectDir: undefined });
  expect(await getHostDetails!("down").then(() => "resolved", () => "rejected")).toBe("rejected");
});

// The card reads its host from a store so a test can change status or identity while a read is held.
function hostStore(initial: Host) {
  let current = initial; const listeners = new Set<() => void>();
  return { get: () => current, set: (next: Host) => { current = next; listeners.forEach((l) => l()); }, subscribe: (l: () => void) => { listeners.add(l); return () => listeners.delete(l); } };
}

function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((r) => { resolve = r; }); return { promise, resolve }; }

const value = (container: HTMLElement, label: string) => [...container.querySelectorAll("dt")].find((dt) => dt.textContent === label)?.nextElementSibling?.textContent;

it("shows details only for the current identity while the host is available", async () => {
  setAppLocale("en-US");
  const base = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
  const store = hostStore(base);
  const reads: { identity: string; done: ReturnType<typeof deferred<HostDetails>> }[] = [];
  const adapter = { ...createDemoAdapter(), getHostDetails: () => { const done = deferred<HostDetails>(); reads.push({ identity: store.get().identity ?? "", done }); return done.promise; } };
  function Harness() { const host = useSyncExternalStore(store.subscribe, store.get); return <HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />; }
  const view = renderWithApp(<Harness />, adapter);
  await waitFor(() => expect(reads.length).toBe(1));

  // Registration rotation while the first read is held: its late answer must never reach the card.
  act(() => store.set({ ...base, identity: `${base.identity}-rotated` }));
  await waitFor(() => expect(reads.length).toBe(2));
  await act(async () => { reads[0]!.done.resolve({ os: "Stale OS", dockerVersion: "1.0" }); });
  expect(value(view.container, "System")).toBe("Not reported by the agent");
  await act(async () => { reads[1]!.done.resolve({ os: "Debian GNU/Linux 13 (trixie)", dockerVersion: "28.5.1" }); });
  await waitFor(() => expect(value(view.container, "System")).toBe("Debian GNU/Linux 13 (trixie)"));
  expect(value(view.container, "Docker")?.includes("28.5.1")).toBe(true);

  // Offline: the cached success is no longer shown, without any new read.
  const before = reads.length;
  act(() => store.set({ ...store.get(), status: "offline" }));
  expect(value(view.container, "System")).toBe("Not reported by the agent");
  expect(reads.length).toBe(before);
});

it("reads the details again after a hosts change with an unchanged identity (Direct reconnect, stream reopen)", async () => {
  setAppLocale("en-US");
  const host = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
  const demo = createDemoAdapter();
  const listeners = new Set<(topics: string[]) => void>();
  let reads = 0;
  const adapter = { ...demo, subscribeChanges: (fn: (topics: string[]) => void) => { listeners.add(fn); return () => listeners.delete(fn); }, getHostDetails: async () => ({ os: `Read ${++reads}`, dockerVersion: "28.0.0" }) };
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />, adapter as typeof demo);
  await waitFor(() => expect(value(view.container, "System")).toBe("Read 1"));
  act(() => listeners.forEach((fn) => fn(["hosts"])));
  await waitFor(() => expect(value(view.container, "System")).toBe("Read 2"));
});

const row = (patch: Record<string, unknown> = {}) => mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false, ...patch } as Parameters<typeof mapHost>[0]);

it.each([
  ["registration rotation", { registrationRevision: 2 }],
  ["Edge session replacement", { isEdge: true, edgeSessionId: "s-2", edgeAgentInstance: "a-1" }],
])("never shows facts read under the previous identity after %s (producer-shaped host rows)", async (_name, patch) => {
  setAppLocale("en-US");
  const base = "isEdge" in patch ? row({ isEdge: true, edgeSessionId: "s-1", edgeAgentInstance: "a-1" }) : row();
  const store = hostStore(base);
  const reads: ReturnType<typeof deferred<HostDetails>>[] = [];
  const adapter = { ...createDemoAdapter(), getHostDetails: () => { const done = deferred<HostDetails>(); reads.push(done); return done.promise; } };
  function Harness() { const host = useSyncExternalStore(store.subscribe, store.get); return <HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />; }
  const view = renderWithApp(<Harness />, adapter);
  await waitFor(() => expect(reads.length).toBe(1));
  act(() => store.set(row({ ...("isEdge" in patch ? { isEdge: true, edgeSessionId: "s-1", edgeAgentInstance: "a-1" } : {}), ...patch })));
  await waitFor(() => expect(reads.length).toBe(2));
  await act(async () => { reads[0]!.resolve({ os: "Previous identity OS" }); });
  expect(value(view.container, "System")).toBe("Not reported by the agent");
  await act(async () => { reads[1]!.resolve({ os: "Current identity OS" }); });
  await waitFor(() => expect(value(view.container, "System")).toBe("Current identity OS"));
});

it.each([["settings"], ["hosts"]] as const)("a %s change during the first read repeats it, so the older answer is never shown", async (topic) => {
  setAppLocale("en-US");
  const local = mapHost({ id: "0", name: "Local Docker", color: null, registrationRevision: 1, apiUrl: "https://hub.example.com", enabled: true, status: "online", isEdge: false });
  const listeners = new Set<(topics: string[]) => void>();
  const reads: ReturnType<typeof deferred<HostDetails>>[] = [];
  const demo = createDemoAdapter();
  const adapter = { ...demo, subscribeChanges: (fn: (topics: string[]) => void) => { listeners.add(fn); return () => listeners.delete(fn); }, getHostDetails: () => { const done = deferred<HostDetails>(); reads.push(done); return done.promise; } };
  const view = renderWithApp(<HostCard host={local} system={undefined} stacks={0} containers={0} allIds={["0"]} />, adapter as typeof demo);
  await waitFor(() => expect(reads.length).toBe(1));
  act(() => listeners.forEach((fn) => fn([topic])));
  // The physical read is not aborted: its older answer is discarded and the read repeats.
  await act(async () => { reads[0]!.resolve({ projectDir: "/srv/old-projects" }); });
  await waitFor(() => expect(reads.length).toBe(2));
  expect(view.container.textContent?.includes("/srv/old-projects")).toBe(false);
  await act(async () => { reads[1]!.resolve({ projectDir: "/srv/new-projects" }); });
  await waitFor(() => expect(value(view.container, "Project directory")).toBe("/srv/new-projects"));
  expect(view.container.textContent?.includes("/srv/old-projects")).toBe(false);
});

it("reads again when an offline host returns with an unchanged identity", async () => {
  setAppLocale("en-US");
  const store = hostStore(row());
  const listeners = new Set<(topics: string[]) => void>();
  let reads = 0;
  const demo = createDemoAdapter();
  const adapter = { ...demo, subscribeChanges: (fn: (topics: string[]) => void) => { listeners.add(fn); return () => listeners.delete(fn); }, getHostDetails: async () => ({ os: `Read ${++reads}` }) };
  function Harness() { const host = useSyncExternalStore(store.subscribe, store.get); return <HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />; }
  const view = renderWithApp(<Harness />, adapter as typeof demo);
  await waitFor(() => expect(value(view.container, "System")).toBe("Read 1"));
  act(() => { store.set(row({ status: "offline" })); listeners.forEach((fn) => fn(["hosts"])); });
  expect(value(view.container, "System")).toBe("Not reported by the agent");
  act(() => { store.set(row()); listeners.forEach((fn) => fn(["hosts"])); });
  await waitFor(() => expect(/^Read [2-9]$/.test(value(view.container, "System") ?? "")).toBe(true));
});

it.each([["cold"], ["warm"]] as const)("a %s card publishes the current answer after more changes in a row than the shared repeat budget", async (start) => {
  setAppLocale("en-US");
  const local = mapHost({ id: "0", name: "Local Docker", color: null, registrationRevision: 1, apiUrl: "https://hub.example.com", enabled: true, status: "online", isEdge: false });
  const listeners = new Set<(topics: string[]) => void>();
  const reads: ReturnType<typeof deferred<HostDetails>>[] = [];
  const demo = createDemoAdapter();
  const adapter = { ...demo, subscribeChanges: (fn: (topics: string[]) => void) => { listeners.add(fn); return () => listeners.delete(fn); }, getHostDetails: () => { const done = deferred<HostDetails>(); reads.push(done); return done.promise; } };
  const view = renderWithApp(<HostCard host={local} system={undefined} stacks={0} containers={0} allIds={["0"]} />, adapter as typeof demo);
  await waitFor(() => expect(reads.length).toBe(1));
  if (start === "warm") {
    await act(async () => { reads[0]!.resolve({ os: "Warm OS" }); });
    await waitFor(() => expect(value(view.container, "System")).toBe("Warm OS"));
    act(() => listeners.forEach((fn) => fn(["hosts"])));
    await waitFor(() => expect(reads.length).toBe(2));
  }
  const first = reads.length - 1;
  // Each read is overtaken by a change before it answers: five times, more than MAX_SUPERSEDED_REPEATS.
  for (let i = 0; i < 5; i++) {
    act(() => listeners.forEach((fn) => fn([i % 2 ? "settings" : "hosts"])));
    await act(async () => { reads[first + i]!.resolve({ os: `Superseded ${i}` }); });
    await waitFor(() => expect(reads.length).toBe(first + i + 2));
    expect(view.container.textContent?.includes("Superseded")).toBe(false);
  }
  await act(async () => { reads[first + 5]!.resolve({ os: "Current OS" }); });
  // Quiet afterwards: the current answer is shown without any further notification and no read is left running.
  await waitFor(() => expect(value(view.container, "System")).toBe("Current OS"));
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(reads.length).toBe(first + 6);
  expect(view.container.textContent?.includes("Superseded")).toBe(false);
});

it.each([["cold"], ["warm"]] as const)("a %s card never keeps an answer superseded between the read's last check and its publication", async (start) => {
  setAppLocale("en-US");
  const host = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "https://agent.example.com", enabled: true, status: "online", isEdge: false });
  const listeners = new Set<(topics: string[]) => void>();
  const reads: ReturnType<typeof deferred<HostDetails>>[] = [];
  const demo = createDemoAdapter();
  const adapter = { ...demo, subscribeChanges: (fn: (topics: string[]) => void) => { listeners.add(fn); return () => listeners.delete(fn); }, getHostDetails: () => { const done = deferred<HostDetails>(); reads.push(done); return done.promise; } };
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />, adapter as typeof demo);
  await waitFor(() => expect(reads.length).toBe(1));
  if (start === "warm") {
    await act(async () => { reads[0]!.resolve({ os: "Warm OS" }); });
    await waitFor(() => expect(value(view.container, "System")).toBe("Warm OS"));
    act(() => listeners.forEach((fn) => fn(["hosts"])));
    await waitFor(() => expect(reads.length).toBe(2));
  }
  const last = reads.length - 1;
  // The change lands in the microtask after the read resolved: after its fence passed, before the query publishes.
  await act(async () => { reads[last]!.resolve({ os: "Superseded at publication" }); queueMicrotask(() => listeners.forEach((fn) => fn(["hosts"]))); });
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(value(view.container, "System") === "Superseded at publication").toBe(false);
  // Quiet afterwards: a follow-up read runs and its answer is shown.
  await waitFor(() => expect(reads.length).toBe(last + 2));
  await act(async () => { reads[last + 1]!.resolve({ os: "Current OS" }); });
  await waitFor(() => expect(value(view.container, "System")).toBe("Current OS"));
});

it.each([["cold", false], ["cold", true], ["warm", false], ["warm", true]] as const)("a %s card reads again when its failing read was overtaken by a change (late handoff: %s)", async (start, late) => {
  setAppLocale("en-US");
  const host = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "https://agent.example.com", enabled: true, status: "online", isEdge: false });
  const listeners = new Set<(topics: string[]) => void>();
  const reads: { promise: Promise<HostDetails>; resolve: (v: HostDetails) => void; reject: (e: unknown) => void }[] = [];
  const demo = createDemoAdapter();
  const adapter = { ...demo, subscribeChanges: (fn: (topics: string[]) => void) => { listeners.add(fn); return () => listeners.delete(fn); }, getHostDetails: () => { let resolve!: (v: HostDetails) => void, reject!: (e: unknown) => void; const promise = new Promise<HostDetails>((a, b) => { resolve = a; reject = b; }); reads.push({ promise, resolve, reject }); return promise; } };
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />, adapter as typeof demo);
  await waitFor(() => expect(reads.length).toBe(1));
  if (start === "warm") {
    await act(async () => { reads[0]!.resolve({ os: "Warm OS" }); });
    await waitFor(() => expect(value(view.container, "System")).toBe("Warm OS"));
    act(() => listeners.forEach((fn) => fn(["hosts"])));
    await waitFor(() => expect(reads.length).toBe(2));
  }
  const last = reads.length - 1;
  const notify = () => listeners.forEach((fn) => fn(["hosts"]));
  if (late) await act(async () => { reads[last]!.reject(new Error("HTTP 502")); queueMicrotask(notify); });
  else { act(notify); await act(async () => { reads[last]!.reject(new Error("HTTP 502")); }); }
  // Quiet afterwards: the change still gets a current read, and its answer is shown.
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(reads.length > last + 1).toBe(true);
  await act(async () => { reads.at(-1)!.resolve({ os: "Current OS" }); });
  await waitFor(() => expect(value(view.container, "System")).toBe("Current OS"));
});

it("does not retry a failing read within the retry interval when no change arrives", async () => {
  setAppLocale("en-US");
  const host = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "https://agent.example.com", enabled: true, status: "online", isEdge: false });
  let reads = 0;
  const demo = createDemoAdapter();
  const adapter = { ...demo, getHostDetails: async () => { reads++; throw new Error("HTTP 502"); } };
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />, adapter as typeof demo);
  await waitFor(() => expect(reads).toBe(1));
  await act(async () => { await new Promise((r) => setTimeout(r, 100)); });
  expect(reads).toBe(1);
  expect(value(view.container, "System")).toBe("Not reported by the agent");
});

it("reads a failed card again after the retry interval while the host stays available, as after an agent restart", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  setAppLocale("en-US");
  const host = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "https://agent.example.com", enabled: true, status: "online", isEdge: false });
  let reads = 0;
  const demo = createDemoAdapter();
  const adapter = { ...demo, getHostDetails: async () => { reads++; if (reads === 1) throw new Error("HTTP 502"); return { os: "Debian GNU/Linux 13 (trixie)", dockerVersion: "28.5.1" }; } };
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={["atlas"]} />, adapter as typeof demo);
  await waitFor(() => expect(reads).toBe(1));
  await waitFor(() => expect(value(view.container, "System")).toBe("Not reported by the agent"));
  await act(async () => { await vi.advanceTimersByTimeAsync(FAILED_READ_RETRY_MS - 1_000); });
  expect(reads).toBe(1);
  await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
  await waitFor(() => expect(value(view.container, "System")).toBe("Debian GNU/Linux 13 (trixie)"));
  expect(reads).toBe(2);
  // A settled answer is not read again on the failure interval.
  await act(async () => { await vi.advanceTimersByTimeAsync(FAILED_READ_RETRY_MS * 2); });
  expect(reads).toBe(2);
});
