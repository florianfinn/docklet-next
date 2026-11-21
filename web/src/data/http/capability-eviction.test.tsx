// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #556 tests: cached capability descriptors end at the same boundaries as their consumers.

import { screen, waitFor } from "@testing-library/react";
import { expect, it } from "vitest";
import { useCapability } from "@/data/use-capability";
import { renderWithApp } from "@/test/render";
import { at, resourceFixture } from "./resources.test-support";

const stack = { kind: "stack" as const, id: "first~project" }, own = { kind: "container" as const, id: "first~web" };
const other = { kind: "container" as const, id: "second~web" }, host = { kind: "host" as const, id: "first" };
const refs = [stack, own, other, host];

function fixture() {
  const f = resourceFixture(), original = f.fetcher.getMockImplementation()!;
  let available = true, hold: Promise<void> | undefined;
  f.fetcher.mockImplementation(async (input) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/auth/me")) return new Response(JSON.stringify({ success: true, data: { id: "operator", username: "operator", requiresPasswordChange: false, isGlobalAdmin: true, locale: "de" } }), { headers: { "content-type": "application/json" } });
    if (url.pathname !== "/api/capabilities" || !["stack", "container", "host"].includes(url.searchParams.get("kind") ?? "")) return original(input);
    const kind = url.searchParams.get("kind") ?? "hub", id = url.searchParams.get("id") ?? "";
    // The answer is fixed when the server receives the read; a held read only arrives later.
    const value = available, wait = hold; if (wait) await wait;
    return new Response(JSON.stringify({ success: true, data: [{ operationId: "runStackCommand", available: value, resourceRef: { kind, id } }] }), { headers: { "content-type": "application/json" } });
  });
  const reads = (ref: { kind: string; id: string }) => f.fetcher.mock.calls.filter(([url]) => { const u = new URL(String(url), "https://hub.example.com"); return u.pathname === "/api/capabilities" && u.searchParams.get("kind") === ref.kind && u.searchParams.get("id") === ref.id; }).length;
  const readAll = () => Promise.all(refs.map((ref) => f.adapter.getCapability("runStackCommand", ref)));
  let run = 0;
  // Each call delivers a new finished run; a repeated terminal of one run is deduplicated by design.
  const terminal = (hostId = "first") => { const id = `run-${++run}`; f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "updated", activityId: id, environmentId: hostId, activity: { id, environmentId: hostId, resourceType: "project", resourceId: "project", status: "success", type: "project_redeploy", endedAt: at } } }), lastEventId: "" } as MessageEvent); };
  return { f, reads, readAll, terminal, setAvailable: (v: boolean) => { available = v; }, setHold: (p?: Promise<void>) => { hold = p; } };
}

async function started() {
  const x = fixture();
  const stop = x.f.adapter.subscribeChanges(() => {});
  await x.f.adapter.listHosts();
  // Descriptors read before the stream's first open stay cached through it.
  await x.readAll();
  x.f.source.onopen?.({} as Event);
  await x.readAll();
  return { ...x, stop };
}

it("a finished stack run reads that stack's and its host's container descriptors again, and nothing else", async () => {
  const { reads, readAll, terminal, stop } = await started();
  try {
    expect(refs.map(reads)).toEqual([1, 1, 1, 1]);
    await Promise.all([readAll(), readAll()]);
    expect(refs.map(reads)).toEqual([1, 1, 1, 1]);
    terminal();
    await Promise.all([readAll(), readAll()]);
    expect(refs.map(reads)).toEqual([2, 2, 1, 1]);
  } finally { stop(); }
});

it("the first stream open keeps the cache; a reopen and a sequence gap read every descriptor again", async () => {
  const { f, reads, readAll, stop } = await started();
  try {
    expect(refs.map(reads)).toEqual([1, 1, 1, 1]);
    f.source.onopen?.({} as Event);
    await readAll();
    expect(refs.map(reads)).toEqual([2, 2, 2, 2]);
    const frame = (id: string) => f.source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "info" } }), lastEventId: id } as MessageEvent);
    frame("1"); frame("2");
    await readAll();
    expect(refs.map(reads)).toEqual([2, 2, 2, 2]);
    frame("4");
    await readAll();
    expect(refs.map(reads)).toEqual([3, 3, 3, 3]);
  } finally { stop(); }
});

it("a read in flight before the eviction never answers later callers", async () => {
  const { f, reads, terminal, setAvailable, setHold, stop } = await started();
  try {
    terminal();
    let release!: () => void;
    setHold(new Promise<void>((resolve) => { release = resolve; }));
    const old = f.adapter.getCapability("runStackCommand", stack);
    await waitFor(() => expect(reads(stack)).toBe(2));
    terminal();
    setHold(undefined); setAvailable(false); release();
    expect((await old).available).toBe(true);
    expect((await f.adapter.getCapability("runStackCommand", stack)).available).toBe(false);
    expect(reads(stack)).toBe(3);
  } finally { stop(); }
});

it("a mounted consumer shows the descriptor changed by a finished run", async () => {
  const x = fixture();
  function Probe() { const c = useCapability("runStackCommand", stack); return <p>{c.pending ? "pending" : String(c.available)}</p>; }
  renderWithApp(<Probe />, x.f.adapter);
  await screen.findByText("true");
  x.f.source.onopen?.({} as Event);
  x.setAvailable(false);
  x.terminal();
  await screen.findByText("false");
});
