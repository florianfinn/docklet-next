// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Explicit runtime refresh updates demanded detail without summary output (#225).

import { expect, it, vi } from "vitest";
import { envelope, response, runtimeDetail, runtimeFixture, RUN_ID, snapshot } from "./runtime-commands.test-support";

it("an explicit runtime refresh updates the existing full detail owner while summaries retain no output", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge");
  f.state.activity = (url) => response({ success: true, data: url.pathname.includes("/edge/") ? [{ ...f.state.run, stepTrace: [] }] : [], pagination: { totalItems: url.pathname.includes("/edge/") ? 1 : 0 } });
  let text = "first output";
  f.state.read = () => envelope(runtimeDetail(f.state.run, { output: [{ outputSeq: 1, timestamp: f.state.run.startedAt, stream: "stdout", text }] }));
  await f.adapter.listActions(); const summary = vi.fn(), owner = vi.fn();
  const off = f.adapter.subscribeActions(summary); const close = f.adapter.observeActionDetail!("edge", RUN_ID, owner); vi.useFakeTimers();
  try {
    await vi.advanceTimersByTimeAsync(0);
    expect(owner.mock.lastCall?.[0]).toMatchObject({ status: "ready", run: { output: [{ text: "first output" }] } });
    text = "manual refresh output"; await f.adapter.getRuntimeRun!("edge", RUN_ID);
    expect(owner.mock.lastCall?.[0]).toMatchObject({ status: "ready", run: { output: [{ text: "manual refresh output" }] } });
    expect(summary.mock.lastCall?.[0].find((run: { id: string }) => run.id === RUN_ID)).toMatchObject({ output: [], log: [] });
  } finally { close(); off(); vi.useRealTimers(); }
});
