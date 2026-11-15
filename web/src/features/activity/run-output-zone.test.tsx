// Source: docklet-next/issues/410
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original hub-zone coverage for run output times.

import { expect, it, vi } from "vitest";
import { renderRun } from "@/data/message-code";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderWithApp } from "@/test/render";
import { RunOutput } from "./RunOutput";
import { outputTime } from "./run-output-text";

vi.mock("@/shared/hooks/use-hub-time-zone", () => ({ useHubTimeZone: () => "Asia/Tokyo" }));

it("formats output times in the given zone with milliseconds", () => {
  expect(outputTime("2026-11-12T12:00:00.123Z", "Asia/Tokyo")).toBe("21:00:00.123");
  expect(outputTime("2026-11-12T23:30:05.007Z", "UTC")).toBe("23:30:05.007");
});

it("run output lines show the hub zone, not the browser zone", () => {
  const at = "2026-11-12T12:00:00.123Z";
  const run = renderRun(mapActivityDetail({ activity: { id: "example-run", type: "project_restart", environmentId: "direct", status: "success", startedAt: at, createdAt: at }, messages: [], output: [{ seq: 1, timestamp: at, stream: "stdout", text: "restarted" }], stepTrace: [], outputTruncated: false } as never), "en-US");
  const view = renderWithApp(<RunOutput run={run} defaultOpen />);
  const time = view.container.querySelector(`time[datetime="${at}"]`);
  expect(time?.textContent).toBe("21:00:00.123");
});
