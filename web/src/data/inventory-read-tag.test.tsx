// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original hook-level proof that automatic inventory reads are background reads (Q-5a, R1E-3).

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import type { Topic } from "./adapter";
import { useStacks } from "./queries";

function Stacks() {
  const stacks = useStacks();
  return <button type="button" onClick={() => void stacks.refetch()}>{stacks.data?.length ?? "-"}</button>;
}

it("R1E-3 mount, invalidation and a refetch of settled data (remount after the stale time, recovery) all read as background, so standby hosts are read", async () => {
  let notify: (topics: Topic[]) => void = () => {};
  const demo = createDemoAdapter();
  const adapter = { ...demo, subscribeChanges: (listener: (topics: Topic[]) => void) => { notify = listener; return () => {}; }, listStacks: vi.fn(async () => []) };
  renderWithApp(<Stacks />, adapter);
  await waitFor(() => expect(adapter.listStacks.mock.calls.length).toBe(1));
  act(() => notify(["stacks"]));
  await waitFor(() => expect(adapter.listStacks.mock.calls.length).toBe(2));
  fireEvent.click(screen.getByRole("button"));
  await waitFor(() => expect(adapter.listStacks.mock.calls.length).toBe(3));
  expect(adapter.listStacks.mock.calls.map((call) => (call as unknown[])[2])).toEqual(["background", "background", "background"]);
});
