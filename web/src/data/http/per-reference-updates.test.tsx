// Source: docklet-next/issues/412
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original Go-produced container DTO to actual update action assertions.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { DockletAdapter } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { ContainerUpdates } from "@/features/containers/ContainerUpdates";
import { renderWithApp } from "@/test/render";
import { hasUpdate } from "@/domain/updates";
import { mapContainer, type ContainerRow } from "./inventory-mapping";
import fixture from "./testdata/per-reference-updates.json";

const rows = fixture as ContainerRow[];

it("F1-4 offers each reference's actual update action from the Go producer DTO", async () => {
  const select = rows.map(() => vi.fn());
  const containers = rows.map((row) => mapContainer(row, "0", [], { stale: false }));
  expect(containers.map((c) => c.update.state)).toEqual(["version", "version"]);
  expect(containers.map((c) => c.image.tag)).toEqual(["stable", "edge"]);
  const adapter: DockletAdapter = {
    ...createDemoAdapter(), kind: "http",
    getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: operationId !== "getAutoUpdatePolicy" }),
    getContainerPreviousImage: async () => undefined,
  };
  renderWithApp(<>{containers.map((c, i) => <ContainerUpdates key={c.id} c={c} select={select[i]!} />)}</>, adapter);
  const actions = await screen.findAllByRole("button", { name: "Container aktualisieren" });
  expect(actions).toHaveLength(2);
  await waitFor(() => expect(actions.every((button) => !(button as HTMLButtonElement).disabled)).toBe(true));
  for (const action of actions) await userEvent.click(action);
  for (const callback of select) expect(callback).toHaveBeenCalledExactlyOnceWith(false);
});

it("never borrows the other reference's producer observation", () => {
  const swapped = rows.map((row, index) => mapContainer({ ...row, updateInfo: rows[1 - index]!.updateInfo }, "0", [], { stale: false }));
  expect(swapped.map((c) => c.update.state)).toEqual(["unchecked", "unchecked"]);
  expect(swapped.map((c) => hasUpdate(c.update))).toEqual([false, false]);
});
