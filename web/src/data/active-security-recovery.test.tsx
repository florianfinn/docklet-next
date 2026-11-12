// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that the mounted Dashboard security source recovers after a transient host failure (R1D-4).

import { act, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { ActiveSecurity, Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { INVENTORY_RECOVERY_MS, inventoryRecovery } from "./inventory-recovery";
import { useActiveSecurity } from "./queries";

afterEach(() => { vi.useRealTimers(); });

function Counts() {
  const active = useActiveSecurity();
  return <p>{active.data?.unavailable.length ?? "-"}</p>;
}

it("R1D-4 the mounted active security query refetches after the recovery interval without navigation or notification", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const host = { id: "first", name: "first", status: "online" } as Host;
  const demo = createDemoAdapter();
  const getActiveSecurity = vi.fn(async (): Promise<ActiveSecurity> => {
    const recovery = inventoryRecovery();
    if (getActiveSecurity.mock.calls.length > 1) return recovery.result({ enabled: true, images: [], unavailable: [] });
    recovery.failed(host, "vulnerability.storage-unavailable");
    return recovery.result({ enabled: true, images: [], unavailable: [{ hostId: "first", code: "vulnerability.storage-unavailable" }] });
  });
  renderWithApp(<Counts />, { ...demo, listHosts: async () => [host], subscribeChanges: () => () => {}, getActiveSecurity });
  await waitFor(() => expect(getActiveSecurity.mock.calls.length).toBe(1));
  await act(async () => { await vi.advanceTimersByTimeAsync(INVENTORY_RECOVERY_MS + 100); });
  await act(async () => { await vi.advanceTimersByTimeAsync(100); });
  expect(getActiveSecurity.mock.calls.length).toBe(2);
});
