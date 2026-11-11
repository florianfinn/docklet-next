// Source: docklet-next/issues/391
// Revision: a331ec9df0c18670fd14d06d043cf58a4ef0fc80
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs of the change-driven refresh owner (F-3, F-8) and the shared automatic backup overview (F-6, F-7).

import { CancelledError } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { createAppQueryClient } from "./app-query-client";
import { MAX_SUPERSEDED_REPEATS, OWN_REFRESH, refresh } from "./refresh-owner";
import { backupFixture } from "./http/backups.test-support";
import { SHARE_MS } from "./http/backups";

afterEach(() => { vi.useRealTimers(); });

it("F-8 a fetch superseded on every attempt repeats at most the bound; the last value stands", async () => {
  const client = createAppQueryClient();
  let calls = 0;
  client.setQueryData(["example"], 0);
  const value = await client.fetchQuery({ queryKey: ["example"], staleTime: 0, queryFn: async () => {
    calls++; refresh(client, [["example"]]); await Promise.resolve(); return calls;
  } });
  expect([calls, value]).toEqual([MAX_SUPERSEDED_REPEATS + 1, MAX_SUPERSEDED_REPEATS + 1]);
});

it("F-3 a fence CancelledError of a superseded attempt repeats; an unsuperseded one fails as before", async () => {
  const client = createAppQueryClient();
  let calls = 0;
  client.setQueryData(["fenced"], "warm"); client.setQueryData(["plain"], "warm");
  const repeated = await client.fetchQuery({ queryKey: ["fenced"], staleTime: 0, queryFn: async () => {
    calls++;
    if (calls === 1) { refresh(client, [["fenced"]]); throw new CancelledError(); }
    return "fresh";
  } }).catch((error: unknown) => (error instanceof CancelledError ? "cancelled" : "failed"));
  const plain = await client.fetchQuery({ queryKey: ["plain"], staleTime: 0, queryFn: async () => { throw new CancelledError(); } }).then(() => "resolved", (error: unknown) => error instanceof CancelledError);
  expect([repeated, calls, plain]).toEqual(["fresh", 2, true]);
});

it("F-7 a cancelled fetch does not repeat after a later supersession", async () => {
  const client = createAppQueryClient();
  let calls = 0, release!: () => void;
  const fetched = client.fetchQuery({ queryKey: ["cancelled"], queryFn: async ({ signal }) => {
    calls++;
    await new Promise<void>((resolve) => { release = resolve; signal.addEventListener("abort", () => resolve(), { once: true }); });
    return calls;
  } }).catch(() => "cancelled");
  await Promise.resolve();
  refresh(client, [["cancelled"]]);
  await client.cancelQueries({ queryKey: ["cancelled"] });
  release();
  expect([await fetched, calls]).toEqual(["cancelled", 1]);
});

it("F-6 background overview reads share one read for the share window; polls, untagged reads and a version bump read again", async () => {
  vi.useFakeTimers();
  const f = backupFixture();
  const reads = () => f.state.requests.filter((r) => r.path.endsWith("/first/volumes/backups/overview")).length;
  await Promise.all([f.api.getBackupOverview("first", "background"), f.api.getBackupOverview("first", "background")]);
  await vi.advanceTimersByTimeAsync(SHARE_MS - 100);
  await f.api.getBackupOverview("first", "background");
  const shared = reads();
  await f.api.getBackupOverview("first", "poll");
  await f.api.getBackupOverview("first");
  const fresh = reads();
  await vi.advanceTimersByTimeAsync(200);
  await f.api.getBackupOverview("first", "background");
  const expired = reads();
  f.invalidate(["first"]);
  await f.api.getBackupOverview("first", "background");
  expect([shared, fresh, expired, reads()]).toEqual([1, 3, 4, 5]);
});

it("F-7 a shared overview read of an earlier identity is not reused; each consumer gets its own copy", async () => {
  const f = backupFixture();
  const first = await f.api.getBackupOverview("first", "background");
  const joined = await f.api.getBackupOverview("first", "background");
  f.rotate();
  await f.api.getBackupOverview("first", "background");
  expect([first === joined, f.state.requests.filter((r) => r.path.endsWith("/first/volumes/backups/overview")).length]).toEqual([false, 2]);
});

it("F-6 a shared read keeps each consumer's budget: one abort leaves it to the other; the last abort ends the read", async () => {
  const f = backupFixture();
  let physical: AbortSignal | undefined, release!: () => void;
  f.intercept(async (url, init) => {
    if (!url.pathname.endsWith("/overview")) return undefined;
    physical = init?.signal ?? undefined;
    await new Promise<void>((resolve) => { release = resolve; init?.signal?.addEventListener("abort", () => resolve(), { once: true }); });
    return undefined;
  });
  const [a, b, c] = [new AbortController(), new AbortController(), new AbortController()];
  const first = f.decorate("first", [], a.signal).then(() => "read", () => "aborted");
  const second = f.decorate("first", [], b.signal).then(() => "read", () => "aborted");
  await vi.waitFor(() => expect(physical).toBeDefined());
  a.abort();
  const afterOne = physical!.aborted;
  release();
  const kept = [await first, await second, afterOne];
  physical = undefined;
  f.invalidate(["first"]);
  const third = f.decorate("first", [], c.signal).then(() => "read", () => "aborted");
  await vi.waitFor(() => expect(physical).toBeDefined());
  c.abort();
  expect([...kept, await third, physical!.aborted]).toEqual(["aborted", "read", false, "aborted", true]);
});

it("F-10 a read fenced by a backup version bump reads its answer to the end instead of cancelling it", async () => {
  const f = backupFixture();
  let entered!: () => void, release!: () => void, cancelled = false, drained = false;
  const reached = new Promise<void>((resolve) => { entered = resolve; }), held = new Promise<void>((resolve) => { release = resolve; });
  f.intercept(async (url) => {
    if (!url.pathname.endsWith("/overview")) return undefined;
    entered(); await held;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) { controller.enqueue(new TextEncoder().encode(JSON.stringify({ success: true, data: {} }))); controller.close(); drained = true; },
      cancel() { cancelled = true; },
    });
    return new Response(body, { headers: { "Content-Type": "application/json" } });
  });
  const read = f.api.getBackupOverview("first").then(() => "read", (error: unknown) => (error instanceof CancelledError ? "cancelled" : "failed"));
  await reached; f.invalidate(["first"]); release();
  expect([await read, cancelled, drained]).toEqual(["cancelled", false, true]);
});

it("a read whose owner coalesces its own refreshes (events) is never superseded by a notification", async () => {
  const client = createAppQueryClient();
  let calls = 0, finish!: (value: string) => void;
  client.setQueryData(["owned"], "warm");
  const refetch = client.fetchQuery({ queryKey: ["owned"], staleTime: 0, meta: OWN_REFRESH, queryFn: () => { calls++; return calls === 1 ? new Promise<string>((resolve) => { finish = resolve; }) : Promise.resolve("repeated"); } });
  await vi.waitFor(() => expect(calls).toBe(1));
  refresh(client, [["owned"]]);
  finish("owner value");
  expect([await refetch, calls]).toEqual(["owner value", 1]);
});
