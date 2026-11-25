// Source: docklet-next/issues/612
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #612 result line regressions for notifications and the Action Centre.

import { act, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { useTranslations } from "use-intl";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp } from "@/test/render";
import { at, securityFixture, terminalRun } from "@/data/http/vulnerabilities.test-support";
import type { ScanRunSummary } from "@/domain/security";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";
import { scanResultLine } from "./scan-counts";

afterEach(() => { cleanup(); activityStore.reset(); setAppLocale("de"); });

const summary = (patch: Partial<ScanRunSummary> = {}): ScanRunSummary => ({ status: "completed", targets: 6, scanned: 6, failed: 0, skipped: 0, counts: { critical: 2, high: 15, medium: 0, low: 0, unknown: 0 }, startedAt: "2026-11-24T20:15:00Z", finishedAt: "2026-11-24T20:15:56Z", ...patch });
function Line({ value }: { value: ScanRunSummary }) { const t = useTranslations("runMessages.scanCounts"); return <p>{scanResultLine(value, t)}</p>; }

it.each([
  ["en-US", summary(), "6 images scanned · 2 critical · 15 high · 56 s"],
  ["de", summary(), "6 Images gescannt · 2 kritisch · 15 hoch · 56 s"],
  ["en-US", summary({ status: "partial", scanned: 4, failed: 2, finishedAt: "2026-11-24T20:19:00Z" }), "4 images scanned · 2 critical · 15 high · 2 failed · partial result · 4 min"],
  ["en-US", summary({ targets: 0, scanned: 0, finishedAt: undefined }), "No images to scan"],
  ["de", summary({ scanned: 1 }), "1 Image gescannt · 2 kritisch · 15 hoch · 56 s"],
] as const)("formats the result line (%s) %#", (locale, value, text) => {
  setAppLocale(locale);
  const view = renderWithApp(<Line value={value} />);
  expect(view.container.textContent).toBe(text);
});

function settledScans(runs: unknown[]) {
  const f = securityFixture();
  f.state.active = true; f.state.finished = true; f.state.runs = runs as typeof f.state.runs;
  return f;
}
const entry = () => document.querySelector("#run-scan-active-first");
const resultOf = () => entry()?.querySelector('[data-slot="scan-result"]')?.textContent;

it("shows the durable result in the collapsed entry and at the top of Details", async () => {
  setAppLocale("en-US");
  const f = settledScans([{ ...terminalRun, activityId: "scan-active-first", status: "partial", targets: 6, scanned: 5, failed: 1 }]);
  await f.adapter.getCurrentUser(); await f.adapter.listHosts();
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  renderWithApp(<ActivityCenter />, f.adapter);
  await waitFor(() => expect(resultOf()).toBe("5 images scanned · 2 critical · 1 high · 1 failed · partial result · 0 s"));
  // Runs of another host without a listed record show no line rather than a borrowed one.
  await waitFor(() => expect(document.querySelector("#run-scan-active-second") !== null).toBe(true));
  expect(document.querySelector('#run-scan-active-second [data-slot="scan-result"]') === null).toBe(true);
  fireEvent.click(entry()!.querySelector("button")!);
  await waitFor(() => expect(entry()?.querySelector('[data-slot="scan-detail-result"]')?.textContent).toContain("1 failed"));
});

// Break-map row 10: a refused list read leaves ordinary activity intact and shows no result.
it("shows no result and no error when the host refuses the run list", async () => {
  setAppLocale("en-US");
  const f = settledScans([{ ...terminalRun, activityId: "scan-active-first" }]), original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => String(input).endsWith("/vulnerabilities/runs") ? new Response(JSON.stringify({ status: 403, code: "vulnerability.permission" }), { status: 403, headers: { "Content-Type": "application/json" } }) : original(input, init));
  await f.adapter.getCurrentUser(); await f.adapter.listHosts();
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  renderWithApp(<ActivityCenter />, f.adapter);
  await waitFor(() => expect(f.fetcher.mock.calls.some(([input]) => String(input).endsWith("/first/vulnerabilities/runs"))).toBe(true));
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 20)); });
  expect(entry()?.textContent).toContain("Succeeded");
  expect(resultOf() === undefined).toBe(true);
  expect(document.querySelector('[role="alert"]') === null).toBe(true);
});

// Break-map row 9: a scan that settles while the centre is open gains its line through the security topic.
it("adds the result after a scan settles while the centre is open", async () => {
  setAppLocale("en-US");
  const f = settledScans([]);
  await f.adapter.getCurrentUser(); await f.adapter.listHosts();
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  renderWithApp(<ActivityCenter />, f.adapter);
  await waitFor(() => expect(entry() !== null).toBe(true));
  await waitFor(() => expect(f.fetcher.mock.calls.some(([input]) => String(input).endsWith("/first/vulnerabilities/runs"))).toBe(true));
  expect(resultOf() === undefined).toBe(true);
  f.state.runs = [{ ...terminalRun, activityId: "scan-active-first", targets: 3, scanned: 3 }];
  await act(async () => {
    f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "update", environmentId: "first", activity: { id: "scan-active-first", type: "vulnerability_scan", environmentId: "first" } } }), lastEventId: "" } as MessageEvent);
  });
  await waitFor(() => expect(resultOf()).toBe("3 images scanned · 2 critical · 1 high · 0 s"));
});

// Break-map row 9 (in flight): a scan that settles while the first list read is still held gains its line.
it("adds the result when the scan settles while the list read is in flight", async () => {
  setAppLocale("en-US");
  const f = settledScans([]), original = f.fetcher.getMockImplementation()!;
  let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  let first = true;
  f.fetcher.mockImplementation(async (input, init) => {
    if (String(input).endsWith("/first/vulnerabilities/runs") && first) { first = false; const answer = original(input, init); await held; return answer; }
    return original(input, init);
  });
  await f.adapter.getCurrentUser(); await f.adapter.listHosts();
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  renderWithApp(<ActivityCenter />, f.adapter);
  await waitFor(() => expect(first).toBe(false));
  f.state.runs = [{ ...terminalRun, activityId: "scan-active-first", targets: 3, scanned: 3 }];
  await act(async () => {
    f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "update", environmentId: "first", activity: { id: "scan-active-first", type: "vulnerability_scan", environmentId: "first" } } }), lastEventId: "" } as MessageEvent);
  });
  await act(async () => { release(); });
  await waitFor(() => expect(resultOf()).toBe("3 images scanned · 2 critical · 1 high · 0 s"));
});
