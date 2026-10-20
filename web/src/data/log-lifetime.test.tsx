// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original hook resource, pause and identity lifetime assertions.

import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { createDisconnectedAdapter } from "./disconnected-adapter";
import { useContainerLogs, useStackLogs } from "./live";
import { AdapterContext } from "./use-adapter";
import { withRunGate } from "./run-gate";
import { logFixture, settleLogs, wireLog } from "./http/logs.test-support";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });
it.each(["stack", "container"] as const)("%s pause closes, retains current view, switches cleanly, and resume replaces the tail", async (kind) => {
  const f = logFixture();
  const adapter = withRunGate({ ...createDisconnectedAdapter(), subscribeLogs: f.owner.subscribeLogs, subscribeContainerLogs: f.owner.subscribeContainerLogs });
  const wrapper = ({ children }: { children: ReactNode }) => <AdapterContext.Provider value={adapter}>{children}</AdapterContext.Provider>;
  const useLogs = kind === "stack" ? useStackLogs : useContainerLogs;
  const view = renderHook(({ id, paused }) => useLogs(id, paused), { wrapper, initialProps: { id: "0~one", paused: false } });
  await act(settleLogs); act(() => { f.sockets[0]!.send(wireLog(1, "old")); f.flush(); });
  expect(view.result.current[0]?.message).toBe("old");
  view.rerender({ id: "0~one", paused: true }); expect(f.sockets[0]!.close).toHaveBeenCalledTimes(1); expect(view.result.current[0]?.message).toBe("old");
  view.rerender({ id: "0~two", paused: true }); expect(view.result.current).toEqual([]);
  view.rerender({ id: "0~two", paused: false }); await act(settleLogs);
  act(() => { f.sockets[1]!.send(wireLog(1, "new")); f.flush(); }); expect(view.result.current[0]?.message).toBe("new");
  view.rerender({ id: "0~two", paused: true });
  act(() => f.owner.reset()); expect(view.result.current).toEqual([]);
  view.rerender({ id: "0~two", paused: false }); await act(settleLogs); expect(view.result.current).toEqual([]);
  view.unmount(); expect(f.frames.size).toBe(0); expect(f.sockets.at(-1)!.close).toHaveBeenCalledTimes(1);
});
it("adapter replacement rejects a captured callback and discards pending frames", async () => {
  const a = logFixture(), b = logFixture();
  const first = { ...createDisconnectedAdapter(), subscribeLogs: a.owner.subscribeLogs };
  const second = { ...createDisconnectedAdapter(), subscribeLogs: b.owner.subscribeLogs };
  let adapter = first;
  const wrapper = ({ children }: { children: ReactNode }) => <AdapterContext.Provider value={adapter}>{children}</AdapterContext.Provider>;
  const view = renderHook(() => useStackLogs("0~same", false), { wrapper }); await act(settleLogs);
  const late = a.sockets[0]!.onmessage!;
  adapter = second; view.rerender(); await act(settleLogs);
  act(() => late(new MessageEvent("message", { data: JSON.stringify(wireLog(1, "stale")) })));
  expect(view.result.current).toEqual([]);
  act(() => { b.sockets[0]!.send(wireLog(1, "current")); b.flush(); });
  expect(view.result.current[0]?.message).toBe("current"); view.unmount();
});
