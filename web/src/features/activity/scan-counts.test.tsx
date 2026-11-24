// Source: docklet-next/issues/583
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import { act, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp } from "@/test/render";
import { readScanProgress } from "@/data/http/vulnerabilities";
import { securityFixture, terminalRun } from "@/data/http/vulnerabilities.test-support";
import { deferred } from "@/data/http/activity-reader.test-support";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun, type CodedActionRun } from "@/data/message-code";
import { useActionRuns } from "@/data/live";
import { ScanProgress } from "@/features/security/ScanProgress";
import { ActionDetail } from "./ActionDetail";
import { ActionToasts } from "./ActionToasts";
import { Toaster } from "@/shared/ui/sonner";
import { toast } from "sonner";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { ActionToastView } from "./ActionToastView";
import { RunFacts } from "./RunFocus";
import { useScanCounts } from "./scan-counts";

afterEach(() => { toast.dismiss(); cleanup(); uiStyleStore.reset(); vi.useRealTimers(); vi.restoreAllMocks(); setAppLocale("de"); });

function Surfaces() {
  const runs = useActionRuns();
  return <><ScanProgress selection={["first"]} />{runs.filter((r) => r.hostId === "first").map((run) => <section key={run.id} data-activity={run.id}>
    <ActionDetail run={run}>{(full) => <RunFacts run={full} now={0} />}</ActionDetail>
    {(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={run} design={design} detail="full" now={0} major onClose={() => {}} onOpen={() => {}} />)}
  </section>)}</>;
}
it.each(["en-US", "de"] as const)("keeps two advancing manual count polls, including failed images, in dialog, toasts and Security (%s)", async (locale) => {
  vi.useFakeTimers(); setAppLocale(locale);
  const f = securityFixture(); f.state.active = true;
  await f.adapter.getCurrentUser(); await f.adapter.listHosts(); await f.adapter.listActions();
  const view = renderWithApp(<Surfaces />, f.adapter);
  await act(async () => {
    await new Promise<void>((resolve) => {
      const off = f.adapter.observeActionDetail!("first", "scan-active-first", (state) => { if (state.status === "ready") { resolve(); queueMicrotask(() => off()); } });
    });
  });
  const text = (done: number, pct: number) => locale === "de" ? `Images: ${done} von 10 gescannt · ${pct} % · 1 fehlgeschlagen` : `Images: ${done} of 10 scanned · ${pct} % · 1 failed`;
  const check = (done: number, pct: number) => {
    expect(view.container.querySelector('[data-scan-activity="scan-active-first"]')?.textContent).toContain(text(done, pct));
    const section = view.container.querySelector('[data-activity="scan-active-first"]')!;
    expect(section.querySelector('[data-fact="progress"] dd')?.textContent).toBe(text(done, pct));
    for (const toast of section.querySelectorAll('[data-slot="action-toast"]')) expect(toast.textContent).toContain(text(done, pct));
  };
  await act(() => vi.advanceTimersByTimeAsync(3_000));
  check(5, 50);
  f.state.progress.scanned = 7;
  await act(() => vi.advanceTimersByTimeAsync(3_000));
  check(9, 90);
  view.unmount();
});
it("preserves scan counts from the durable read including zero targets and rejects missing counts", async () => {
  const request = async () => ({ data: { ...terminalRun, activityId: "example", targets: 0, scanned: 0, skipped: 0, failed: 0 } });
  expect(await readScanProgress(request, "first", "example")).toEqual({ targets: 0, done: 0, failed: 0, summary: expect.objectContaining({ status: "completed", targets: 0, scanned: 0 }) });
  for (const field of ["targets", "scanned", "skipped", "failed"]) {
    await expect(readScanProgress(async () => ({ data: { ...terminalRun, activityId: "example", [field]: undefined } }), "first", "example")).rejects.toThrow();
  }
});
it.each([undefined, { targets: 0, done: 0, failed: 0 }])("does not invent a ratio from unavailable or empty scan counts %#", (scan) => {
  const coded = mapActivityDetail({ activity: { id: "example", environmentId: "0", type: "vulnerability_scan", status: "running", startedAt: terminalRun.startedAt, createdAt: terminalRun.startedAt }, output: [], stepTrace: [], messages: [], outputTruncated: false });
  const run = { ...renderRun(coded, "de"), scan };
  function Surface() { const counts = useScanCounts(run); return <p>{counts ?? "unavailable"}</p>; }
  const view = renderWithApp(<Surface />);
  expect(view.container.textContent).toBe("unavailable");
  expect(view.container.textContent).not.toMatch(/NaN|Infinity|0 of 0|0 von 0/);
});

const at = terminalRun.startedAt;
const candidate = { activityId: "older", trigger: "automatic", status: "partial", startedAt: at, targets: 8, scanned: 2, cached: 1, deferred: 2, failed: 1 };
function candidateFixture(status = "success") {
  const f = securityFixture(), original = f.fetcher.getMockImplementation()!;
  let state = status, missing = false;
  const detail = (id: string, host: string) => ({ activity: { id, type: "vulnerability.candidate-scan", environmentId: host, resourceType: "environment", resourceId: "0", status: state, startedAt: at, createdAt: at }, output: [], stepTrace: [], messages: [], outputTruncated: false });
  f.fetcher.mockImplementation(async (input, init) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const host = path.split("/")[3]!;
    if (path.endsWith("/activities")) return Response.json({ success: true, data: host === "first" ? [detail("older", host).activity, detail("newer", host).activity] : [], pagination: { totalItems: host === "first" ? 2 : 0 } });
    if (/\/activities\/(older|newer)$/.test(path)) return Response.json({ success: true, data: detail(path.split("/").at(-1)!, host) });
    if (/\/vulnerabilities\/candidates\/runs\//.test(path)) return missing ? Response.json({ status: 404, code: "vulnerability.run-not-found" }, { status: 404 }) : Response.json({ success: true, data: path.endsWith("/older") ? candidate : { ...candidate, activityId: "newer", scanned: 7, targets: 99 } });
    if (path.endsWith("/vulnerabilities/candidates")) return Response.json({ success: true, data: { run: { ...candidate, activityId: "newer", scanned: 7, targets: 99 } } });
    return original(input, init);
  });
  return { ...f, setStatus: (value: string) => { state = value; }, miss: () => { missing = true; } };
}
async function read(f: ReturnType<typeof candidateFixture>, host = "first", id = "older") {
  return new Promise<CodedActionRun>((resolve, reject) => {
    let off = () => {};
    off = f.adapter.observeActionDetail!(host, id, (state) => {
      if (state.status === "ready") { resolve(state.run); queueMicrotask(() => off()); }
      if (state.status === "error") { reject(state.error); queueMicrotask(() => off()); }
    });
  });
}
it("loads candidate final counts by this activity and host on every fresh adapter, independent of latest or current images", async () => {
  for (let reload = 0; reload < 2; reload++) {
    const f = candidateFixture(); await f.adapter.getCurrentUser();
    const older = await read(f), newer = await read(f, "first", "newer");
    expect(older.candidateScan).toMatchObject({ targets: 8, scanned: 2, cached: 1, deferred: 2, failed: 1 });
    expect(older.candidateScan).not.toHaveProperty("done");
    expect(newer.candidateScan?.targets).toBe(99);
    const paths = f.fetcher.mock.calls.map(([url]) => String(url));
    expect(paths.some((p) => p.endsWith("/environments/first/vulnerabilities/candidates/runs/older"))).toBe(true);
    expect(paths.some((p) => p.endsWith("/vulnerabilities/candidates") || /\/images/.test(p))).toBe(false);
    const view = renderWithApp(<RunFacts run={renderRun(older, "de")} now={0} />, f.adapter);
    expect(view.container.querySelector('[data-fact="progress"] dd')?.textContent).toBe("2 gescannt, 1 aus dem Cache bei 8 Zielen · 2 zurückgestellt · 1 fehlgeschlagen");
    view.unmount();
  }
});
it("a candidate retention miss shows only its result; a running candidate never reads final counts", async () => {
  const f = candidateFixture("running"); await f.adapter.getCurrentUser();
  const running = await read(f);
  expect(running.candidateScan).toBeUndefined();
  expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/candidates/runs/"))).toBe(false);
  const view = renderWithApp(<RunFacts run={renderRun(running, "de")} now={0} />, f.adapter);
  expect(view.container.querySelector('[data-fact="progress"] dd')?.textContent).toBe("Läuft");
  view.unmount();
  const missing = candidateFixture(); missing.miss(); await missing.adapter.getCurrentUser();
  const terminal = await read(missing);
  expect(terminal.candidateScan).toBeUndefined();
  const output = renderWithApp(<RunFacts run={renderRun(terminal, "de")} now={0} />, missing.adapter);
  expect(output.container.textContent).not.toContain("gescannt");
});
it("discards candidate counters held across a host registration replacement", async () => {
  const f = candidateFixture(); await f.adapter.getCurrentUser(); await f.adapter.listHosts();
  const held = deferred<Response>(), started = deferred<void>(), original = f.fetcher.getMockImplementation()!;
  let first = true;
  f.fetcher.mockImplementation((input, init) => {
    if (String(input).endsWith("/candidates/runs/older")) {
      if (first) { first = false; started.resolve(); return held.promise; }
      return Promise.resolve(Response.json({ success: true, data: { ...candidate, scanned: 4 } }));
    }
    return original(input, init);
  });
  const received: number[] = [], ready = deferred<void>();
  const off = f.adapter.observeActionDetail!("first", "older", (state) => {
    if (state.status === "ready") { received.push(state.run.candidateScan?.scanned ?? -1); ready.resolve(); }
  });
  await started.promise;
  f.inventory.state.registrationRevision = 2;
  f.source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: at, environment: { environments: ["first", "second"].map((id) => ({ id, registrationRevision: 2, apiUrl: "http://agent.example.com", isEdge: false, status: "online", enabled: true })) } }) } as MessageEvent);
  held.resolve(Response.json({ success: true, data: candidate }));
  await ready.promise;
  expect(received).not.toContain(2);
  expect(received).toContain(4);
  off();
});

it("a visible toast owns live scan reads after leaving Security and releases them on unmount", async () => {
  vi.useFakeTimers(); setAppLocale("en-US"); setUiStyle({ actionPresentationOverride: "toast" });
  const f = securityFixture();
  await f.adapter.getCurrentUser(); await f.adapter.listHosts(); await f.adapter.listActions();
  const view = renderWithApp(<><ActionToasts /><Toaster /></>, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(0));
  await act(async () => { await f.adapter.runScan({ hostIds: ["first"] }); await f.adapter.listActions(); });
  await act(() => vi.advanceTimersByTimeAsync(32));
  const visible = () => document.querySelector('[data-slot="action-toast"]');
  expect(visible() !== null).toBe(true);
  expect(visible()?.textContent).toContain("Images: 5 of 10 scanned · 50 % · 1 failed");
  expect(document.querySelector('[data-scan-activity]') === null).toBe(true);
  f.state.progress.scanned = 7;
  await act(() => vi.advanceTimersByTimeAsync(3_000));
  expect(visible()?.textContent).toContain("Images: 9 of 10 scanned · 90 % · 1 failed");
  view.unmount();
  const reads = f.fetcher.mock.calls.filter(([url]) => String(url).includes("/vulnerabilities/runs/scan-active")).length;
  await act(() => vi.advanceTimersByTimeAsync(9_000));
  expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/vulnerabilities/runs/scan-active")).length).toBe(reads);
});

it("loads candidate counts only at the terminal transition of the observed activity", async () => {
  vi.useFakeTimers();
  const f = candidateFixture("running");
  await f.adapter.getCurrentUser(); await f.adapter.listHosts(); await f.adapter.listActions();
  const initial = deferred<void>(), final = deferred<CodedActionRun>();
  const off = f.adapter.observeActionDetail!("first", "older", (state) => {
    if (state.status !== "ready") return;
    if (state.run.status === "running") initial.resolve(); else final.resolve(state.run);
  });
  await initial.promise;
  expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/candidates/runs/"))).toBe(false);
  f.setStatus("success");
  await vi.advanceTimersByTimeAsync(3_000);
  const terminal = await final.promise;
  expect(terminal.candidateScan).toMatchObject({ targets: 8, scanned: 2, cached: 1, deferred: 2, failed: 1 });
  off();
});
