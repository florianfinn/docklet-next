// Source: docklet-next/web/src/features/activity/ActionDetail.tsx
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original regression: a background detail refresh must not insert a loading line under shown details.

import { act, cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { ActionDetailState } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";
import { ActionDetail } from "./ActionDetail";

afterEach(cleanup);

it("keeps shown details stable while a background refresh runs", async () => {
  const at = "2026-11-25T10:00:00Z";
  const run = mapActivityDetail({ activity: { id: "scan-1", environmentId: "0", type: "vulnerability_scan", resourceType: "environment", resourceId: "0", status: "running", startedAt: at, createdAt: at }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
  const adapter = createDemoAdapter();
  let push!: (state: ActionDetailState) => void;
  adapter.observeActionDetail = (_host, _id, listener) => { push = listener; return () => {}; };
  renderWithApp(<ActionDetail run={renderRun(run, "en-US")}>{(shown) => <p>{`body ${shown.id}`}</p>}</ActionDetail>, adapter);
  expect(screen.queryByRole("status") !== null).toBe(true);
  await act(async () => { push({ status: "ready", run }); });
  expect(screen.queryByText("body scan-1") !== null).toBe(true);
  await act(async () => { push({ status: "ready", run, refreshing: true }); });
  expect(screen.queryByText("body scan-1") !== null).toBe(true);
  expect(screen.queryByRole("status") === null).toBe(true);
});
