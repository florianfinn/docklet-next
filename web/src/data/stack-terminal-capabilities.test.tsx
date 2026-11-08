// Source: docklet-next/issues/383
// Revision: 76b2f1f2d11ab556003a2c3bde321a618c10ad14
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original stack terminal capability invalidation check.

import { act, render } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { expect, it } from "vitest";
import { AdapterProvider } from "./AdapterProvider";
import { createDemoAdapter } from "./demo/demo-adapter";

// A stack terminal refreshes the stack's own descriptors and the container
// descriptors of its host, and leaves other hosts' descriptors cached.
it("stack terminal invalidates the stack and its host's container capabilities only", () => {
  let listener: ((target: { stackId: string; hostId: string }) => void) | undefined;
  const adapter = { ...createDemoAdapter(), subscribeStackTerminals: (next: typeof listener) => { listener = next; return () => { listener = undefined; }; } };
  let client: QueryClient | undefined;
  function Probe() { client = useQueryClient(); return null; }
  const view = render(<AdapterProvider adapter={adapter}><Probe /></AdapterProvider>);
  const keys = {
    stack: ["capability", "runStackCommand", "stack", "0~project"],
    otherStack: ["capability", "runStackCommand", "stack", "0~other"],
    hostContainer: ["capability", "recreateContainer", "container", "0~c1"],
    otherHostContainer: ["capability", "recreateContainer", "container", "first~c1"],
  } as const;
  for (const key of Object.values(keys)) client!.setQueryData(key, { available: true });
  act(() => listener!({ stackId: "0~project", hostId: "0" }));
  const invalidated = (key: readonly unknown[]) => client!.getQueryState(key)?.isInvalidated === true;
  expect([invalidated(keys.stack), invalidated(keys.hostContainer), invalidated(keys.otherStack), invalidated(keys.otherHostContainer)]).toEqual([true, true, false, false]);
  view.unmount();
});
