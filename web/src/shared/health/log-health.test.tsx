// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { LogsApi } from "@/data/api";
import { renderRoutes } from "@/test/render";
import { HealthDetails } from "./HealthDetails";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => setAppLocale("de"));
it("aborts a held read on identity change and never paints A lines under B", async () => {
  const user = userEvent.setup();
  const requests: { id: string; sink: Parameters<LogsApi["readLogs"]>[2]; signal: AbortSignal }[] = [];
  const readLogs: LogsApi["readLogs"] = vi.fn<LogsApi["readLogs"]>(async (ref, _options, sink, signal) => {
    requests.push({ id: ref.id, sink, signal });
    return { reason: "end", lines: 0, bytes: 0 };
  });
  function Changing() {
    const [id, setId] = useState("A");
    return <><button onClick={() => setId("B")}>change</button><HealthDetails containerId={id} state="stopped" /></>;
  }
  renderRoutes([{ path: "/", element: <Changing /> }], ["/"], { ...createDemoAdapter(), readLogs });
  await waitFor(() => expect(requests).toHaveLength(1));
  await user.click(screen.getByText("change"));
  await waitFor(() => expect(requests).toHaveLength(2));
  await act(async () => requests[0]!.sink([{ id: "a", service: "web", ts: "2026-01-01T00:00:00Z", level: "info", message: "A private result" }]));
  expect(screen.queryByText("A private result") === null).toBe(true);
  expect(requests[0]!.signal.aborted).toBe(true);
  await act(async () => requests[1]!.sink([{ id: "b", service: "web", ts: "2026-01-01T00:00:00Z", level: "info", message: "B result" }]));
  expect(screen.getByText("B result").textContent).toBe("B result");
});
it("shows unavailable with retry and silently accepts a capped read", async () => {
  const user = userEvent.setup();
  const readLogs = vi.fn<LogsApi["readLogs"]>().mockResolvedValueOnce({ reason: "unavailable", lines: 0, bytes: 0 }).mockResolvedValue({ reason: "cap-lines", lines: 20, bytes: 20 });
  renderRoutes([{ path: "/", element: <HealthDetails containerId="example" state="stopped" /> }], ["/"], { ...createDemoAdapter(), readLogs });
  await screen.findByText("Logs not available for this container");
  await user.click(screen.getByRole("button", { name: "Retry" }));
  await waitFor(() => expect(screen.queryByText("Logs not available for this container") === null).toBe(true));
  expect(readLogs).toHaveBeenCalledTimes(2);
});
it("a read cut at the byte cap says so; the 20-line cap stays silent", async () => {
  const readLogs = vi.fn<LogsApi["readLogs"]>().mockResolvedValue({ reason: "cap-bytes", lines: 3, bytes: 64 * 1024 });
  renderRoutes([{ path: "/", element: <HealthDetails containerId="example" state="stopped" /> }], ["/"], { ...createDemoAdapter(), readLogs });
  await waitFor(() => expect(readLogs).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.queryByText("Long lines were cut off after 64 KiB.") !== null).toBe(true));
});
it("on the container page the link reveals the log card on the same page", async () => {
  const user = userEvent.setup();
  const onOpenLogs = vi.fn();
  const readLogs = vi.fn<LogsApi["readLogs"]>().mockResolvedValue({ reason: "end", lines: 0, bytes: 0 });
  renderRoutes([{ path: "/", element: <HealthDetails containerId="example" state="stopped" onOpenLogs={onOpenLogs} /> }], ["/"], { ...createDemoAdapter(), readLogs });
  await user.click(await screen.findByRole("button", { name: "Open in Logs tab" }));
  expect(onOpenLogs).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("link", { name: "Open in Logs tab" }) === null).toBe(true);
});
it("a running healthy container shows no restart line", async () => {
  renderRoutes([{ path: "/", element: <HealthDetails containerId="example" state="running" condition={{ containerId: "example", condition: "running", reason: "", sampledAt: "2026-01-01T00:00:00Z" }} /> }], ["/"], { ...createDemoAdapter(), readLogs: vi.fn<LogsApi["readLogs"]>() });
  expect(screen.queryByText("Restarts in 10 minutes: 0") === null).toBe(true);
});
it("running containers start no read and operator stops show zero restarts", async () => {
  const readLogs = vi.fn<LogsApi["readLogs"]>();
  renderRoutes([{ path: "/", element: <HealthDetails containerId="example" state="running" condition={{ containerId: "example", condition: "stopped", reason: "operator-stop", sampledAt: "2026-01-01T00:00:00Z" }} /> }], ["/"], { ...createDemoAdapter(), readLogs });
  expect(screen.getByText("Container stopped by the operator.").textContent).toBeTruthy();
  expect(screen.getByText("Restarts in 10 minutes: 0").textContent).toBeTruthy();
  expect(readLogs).not.toHaveBeenCalled();
});
it("an interrupted read says lines may be missing and offers a retry", async () => {
  const readLogs = vi.fn<LogsApi["readLogs"]>().mockResolvedValue({ reason: "interrupted", lines: 3, bytes: 30 });
  renderRoutes([{ path: "/", element: <HealthDetails containerId="example" state="stopped" /> }], ["/"], { ...createDemoAdapter(), readLogs });
  await waitFor(() => expect(readLogs).toHaveBeenCalledTimes(1));
  await act(async () => { for (let i = 0; i < 12; ++i) await Promise.resolve(); });
  expect([screen.queryByText("Reading the logs was interrupted; lines may be missing.") !== null, screen.queryByRole("button", { name: "Retry" }) !== null]).toEqual([true, true]);
});
