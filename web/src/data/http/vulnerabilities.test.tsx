// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { isCancelledError, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderRoutes, renderWithApp } from "@/test/render";
import SecurityPage from "@/features/security/SecurityPage";
import SettingsPage from "@/features/settings/SettingsPage";
import { ImagesTab } from "@/features/security/SecurityTabs";
import { securityFixture, terminalRun } from "./vulnerabilities.test-support";
import { failure, finding, run, summaries } from "./vulnerability-decoders";
import { deferred } from "./activity-reader.test-support";

afterEach(() => { setAppLocale("de"); localStorage.clear(); });
it("keeps the same image ID on two hosts as two image rows", async () => {
  const f = securityFixture(), security = await f.adapter.getSecurity();
  expect(security.images.filter((i) => i.id?.endsWith("~sha256:user")).map((i) => i.id)).toEqual(["first~sha256:user", "second~sha256:user"]);
  renderWithApp(<ImagesTab images={security.images} stacks={[]} hosts={new Map()} />, f.adapter);
  const table = screen.getByRole("table", { name: "Images" });
  expect(within(table).getAllByRole("row").filter((r) => r.textContent?.includes("example:1.0.0"))).toHaveLength(2);
});
it("unavailable host reads render unknown counts rather than zero findings", async () => {
  const f = securityFixture(); f.state.failureHost = "second";
  const security = await f.adapter.getSecurity();
  expect(security.unavailable).toEqual([{ hostId: "second", code: "vulnerability.storage-unavailable" }]);
  expect(security.images.find((i) => i.hostId === "first")?.counts.critical).toBe(2);
  renderWithApp(<ImagesTab images={security.images.filter((i) => i.hostId === "second")} stacks={[]} hosts={new Map()} />, f.adapter);
  const rows = within(screen.getByRole("table", { name: "Images" })).getAllByRole("row").slice(2);
  expect(rows.length).toBeGreaterThan(0);
  expect(rows.every((r) => [...r.querySelectorAll("td")].slice(4, 9).every((td) => td.textContent === "–"))).toBe(true);
});
it("retains complete host batches while a slower pending host refetch is delayed", async () => {
  const f = securityFixture();
  let client!: QueryClient;
  function Probe() { client = useQueryClient(); return null; }
  renderRoutes([{ path: "/security", element: <><SecurityPage /><Probe /></> }], ["/security?tab=images"], f.adapter);
  await screen.findByRole("table", { name: "Images" });
  await waitFor(() => expect(screen.getAllByText("example:1.0.0")).toHaveLength(2));
  const delayed = deferred<void>(); f.state.delay = delayed.promise; f.inventory.state.status = "pending";
  let refreshing!: Promise<void>;
  act(() => { refreshing = client.invalidateQueries({ queryKey: ["security"] }); });
  await waitFor(() => expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/second/images/vulnerabilities/summaries")).length).toBeGreaterThan(1));
  expect(screen.getAllByText("example:1.0.0")).toHaveLength(2);
  await act(async () => { delayed.resolve(); await refreshing; });
  expect(screen.getAllByText("example:1.0.0")).toHaveLength(2);
});
it("busy 409 follows and shows the running activity rather than an admission error", async () => {
  const f = securityFixture(); f.state.busy = true;
  renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=images"], f.adapter);
  const button = await screen.findByRole("button", { name: "Jetzt scannen" });
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(button);
  await waitFor(() => expect(document.querySelector('[data-scan-activity^="scan-active-"]') !== null).toBe(true));
  expect(screen.queryByText("Scan konnte nicht gestartet werden. Angenommene Hosts sind in der Aktivität sichtbar.") === null).toBe(true);
});
it("narrows host scans and scopes per-image commands without a per-request policy override", async () => {
  const f = securityFixture();
  expect(await f.adapter.runScan({ hostIds: ["second"] })).toBe("scan-active-second");
  expect(f.fetcher.mock.calls.some(([url]) => String(url).endsWith("/first/vulnerabilities/scan"))).toBe(false);
  await f.adapter.runScan({ imageId: "first~sha256:user" });
  const call = f.fetcher.mock.calls.find(([url]) => String(url).includes("/images/sha256%3Auser/vulnerabilities/scan"));
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({ origin: "manual" });
});
it("rejects undeclared vulnerability error codes and validates both failure shapes", () => {
  expect(() => failure({ status: 409, code: "vulnerability.undeclared", activityId: "run" }, 409)).toThrow();
  expect(failure({ status: 409, code: "vulnerability.busy", activityId: "run" }, 409)).toEqual({ code: "vulnerability.busy", activityId: "run" });
  expect(failure({ status: 403, title: "Forbidden", errors: [] }, 403).code).toBe("vulnerability.permission");
  expect(() => run({ ...terminalRun, errorCode: "vulnerability.undeclared" })).toThrow();
});
it("rejects negative counts, missing identities, wrong enums and malformed bounded DTOs", () => {
  for (const changed of [{ critical: -1 }, { activityId: "" }, { activityId: "x".repeat(257) }, { origin: "other" }, { status: "other" }]) expect(() => run({ ...terminalRun, ...changed })).toThrow();
  expect(() => finding({ severity: "NEW", imageId: "image" }, "first")).toThrow();
  expect(() => summaries({ summaries: { image: { imageId: "other" } } })).toThrow();
});
it("reads host-wide terminal history with interrupted runs excluded from numeric samples", async () => {
  const f = securityFixture(); f.state.runs = [{ ...terminalRun, status: "interrupted", scanned: 0 }];
  const data = await f.adapter.getSecurity();
  expect(data.history.map((r) => [r.hostId, r.status, r.sampled])).toEqual([["first", "interrupted", false], ["second", "interrupted", false]]);
});
it("saves independent switches and notification choices and uses the backend next run", async () => {
  const f = securityFixture(); setAppLocale("en-US");
  renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/security"], f.adapter);
  await screen.findByRole("switch", { name: "Automatic scans" });
  expect(screen.getByText(/Next run:/).textContent).toContain("2026");
  await userEvent.click(screen.getByRole("switch", { name: "Automatic scans" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "New critical findings" }));
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(f.state.settings.vulnerabilityScanEnabled).toBe("true"));
  expect(f.state.settings.vulnerabilityNotifyCritical).toBe("true");
  expect(f.state.settings.featureVulnerabilityManagementEnabled).toBe("true");
});

it("the production run gate sends repeated and independent scan admissions to the host", async () => {
  const { withRunGate } = await import("../run-gate");
  const f = securityFixture(), adapter = withRunGate(f.adapter);
  await adapter.runScan({ hostIds: ["first"] });
  await adapter.runScan({ hostIds: ["second"] });
  await adapter.runScan({ hostIds: ["first"] });
  expect(f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/vulnerabilities/scan"))).toHaveLength(3);
});
it.each([390, 1280])("keeps the scan controls and findings usable at %s px with complete English catalogues", async (width) => {
  const happy = window as unknown as { happyDOM: { setViewport: (v: { width: number; height: number }) => void } };
  act(() => happy.happyDOM.setViewport({ width, height: 800 }));
  setAppLocale("en-US"); const f = securityFixture();
  renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=findings"], f.adapter);
  await screen.findByRole("table", { name: "Vulnerabilities" });
  expect(screen.getAllByText("Known exploited")).toHaveLength(2);
  expect(screen.getAllByText("23.00%")).toHaveLength(2);
  expect(screen.getAllByText("Unknown").length).toBeGreaterThan(0);
  expect(screen.getByRole("button", { name: "Scan now" }) !== null).toBe(true);
  expect(screen.getByRole("table", { name: "Vulnerabilities" }).parentElement?.className).toContain("overflow-x-auto");
  act(() => happy.happyDOM.setViewport({ width: 1280, height: 800 }));
});

it("excludes ignored findings from live image counts and dashboard actionable consumers", async () => {
  const f = securityFixture(); f.state.ignoreCritical = true;
  const data = await f.adapter.getSecurity();
  expect(data.images.find((i) => i.id === "first~sha256:user")?.counts).toEqual({ critical: 0, high: 1, medium: 0, low: 0, unknown: 3 });
  expect(data.findings.some((r) => r.severity === "critical")).toBe(false);
});
it.each([false, true])("keeps retained security inventory from pending hosts (Edge=%s)", async (isEdge) => {
  const f = securityFixture(); f.inventory.state.status = "pending"; f.inventory.state.isEdge = isEdge;
  const data = await f.adapter.getSecurity();
  expect(data.images.filter((i) => i.id?.endsWith("~sha256:user")).map((i) => i.hostId)).toEqual(["first", "second"]);
});

it("cancels through the returned activity and refreshes findings after it settles", async () => {
  const f = securityFixture(); f.state.active = true;
  renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=images"], f.adapter);
  await screen.findByRole("table", { name: "Images" });
  const buttons = await screen.findAllByRole("button", { name: "Scan abbrechen" });
  const before = f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/images/vulnerabilities/summaries")).length;
  const cancelledId = buttons[0]!.closest("[data-scan-activity]")!.getAttribute("data-scan-activity");
  f.state.ignoreCritical = true;
  await userEvent.click(buttons[0]!);
  await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/activities/scan-active-") && String(url).endsWith("/cancel"))).toBe(true));
  await waitFor(() => expect(f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/images/vulnerabilities/summaries")).length).toBeGreaterThan(before));
  await waitFor(() => expect(document.querySelector(`[data-scan-activity="${cancelledId}"]`) === null).toBe(true));
});
it("does not render a zero-valued overview for exclusively unavailable hosts", async () => {
  const f = securityFixture(); f.inventory.state.hostIds = ["first"]; f.state.failureHost = "first";
  renderWithApp(<SecurityPage />, f.adapter);
  await screen.findByText("Scan-Daten nicht verfügbar");
  expect(screen.queryByRole("img", { name: /Risiko/ }) === null).toBe(true);
});

it("discards a late vulnerability response after identity replacement and releases its transport", async () => {
  const { vulnerabilityTransport } = await import("./vulnerability-transport");
  const late = deferred<Response>(); let version = 0; let signal: AbortSignal | undefined;
  const request = vulnerabilityTransport("/api", { token: async () => "example", fetch: (async (_input, init) => { signal = init?.signal ?? undefined; return late.promise; }) as typeof fetch }, () => version);
  const pending = request("/environments/first/vulnerabilities/runs");
  const refusal = pending.catch((error: unknown) => error);
  version++;
  late.resolve(Response.json({ success: true, data: [] }));
  expect(isCancelledError(await refusal)).toBe(true);
  expect(signal?.aborted).toBe(true);
});

it("recovers an unavailable host's security data on an environment state notification", async () => {
  const f = securityFixture(); f.state.failureHost = "second";
  renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=images"], f.adapter);
  await screen.findByText("Scan-Daten für second nicht verfügbar. Ergebnisse anderer Hosts bleiben verfügbar.");
  f.state.failureHost = "";
  act(() => f.source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: "2026-10-27T03:00:00Z", environment: { type: "updated" } }), lastEventId: "1" } as MessageEvent));
  await waitFor(() => expect(screen.queryByText("Scan-Daten für second nicht verfügbar. Ergebnisse anderer Hosts bleiben verfügbar.") === null).toBe(true));
  expect(screen.getAllByText("example:1.0.0")).toHaveLength(2);
});
