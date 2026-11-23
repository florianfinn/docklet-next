// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original host detail read and card proofs for M37 #579.

import { act, waitFor } from "@testing-library/react";
import { useSyncExternalStore } from "react";
import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { createHostDetails } from "@/data/http/host-details";
import type { CsrfClient } from "@/data/http/csrf-client";
import { mapHost } from "@/data/http/hosts";
import type { Host, HostDetails } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { HostCard } from "./HostCard";

afterEach(() => { setAppLocale("de"); });

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
  expect(value(view.container, "Agent / Docker")?.includes("28.5.1")).toBe(true);

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
