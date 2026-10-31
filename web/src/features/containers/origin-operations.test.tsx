// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W1/W2 real-gesture and backend authority regression tests.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { mapContainer, type ContainerRow } from "@/data/http/inventory-mapping";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import { ContainerProvenance } from "./ContainerProvenance";
import { FleetRing } from "../dashboard/FleetRing";
import { fleetUnits } from "../dashboard/fleet-status";

const base = STANDALONE[0]!;
afterEach(() => { preferencesStore.reset(); localStorage.clear(); });

it("W1 uses the backend origin and source, with unknown as an older-data fallback", () => {
  const row = { id: "example", names: ["/app"], state: "running", status: "Up", image: "example/app:1", labels: { "com.docker.compose.project": "app", "com.docker.compose.service": "api" }, origin: "unraid", originSource: "manual" } as unknown as ContainerRow;
  const observation = { stale: false } as Parameters<typeof mapContainer>[3];
  expect(mapContainer(row, "0", [], observation)).toMatchObject({ origin: "unraid", originSource: "manual" });
  expect(mapContainer({ ...row, origin: undefined, originSource: undefined } as unknown as ContainerRow, "0", [], observation).origin).toBe("unknown");
});

it("W1 hides Unraid ring segments by default and persists the personal toggle", async () => {
  const host = HOSTS.find((item) => item.status === "online")!;
  const units = fleetUnits([host], [], [{ ...base, id: "unraid", hostId: host.id, origin: "unraid" }]);
  const view = () => <FleetRing hosts={[host]} units={units} label="Fleet" center={{ value: "1", caption: "" }} />;
  const rendered = renderWithApp(view());
  expect(preferencesStore.get().showUnraidRing).toBe(false);
  expect(rendered.container.querySelectorAll("path[data-runtime]").length).toBe(0);
  await userEvent.click(screen.getByRole("checkbox", { name: "Unraid-Container anzeigen" }));
  expect(rendered.container.querySelectorAll("path[data-runtime]").length).toBe(1);
  expect(JSON.parse(localStorage.getItem("docklet-react-opus.preferences")!).data.showUnraidRing).toBe(true);
  rendered.unmount();
  const again = renderWithApp(view());
  expect(again.container.querySelectorAll("path[data-runtime]").length).toBe(1);
});

it("W2 excludes standalone for any partial Compose label and confirms the current target", async () => {
  const adapter = createDemoAdapter();
  const assign = vi.spyOn(adapter, "setContainerOrigin").mockResolvedValue();
  const c = { ...base, id: base.id, origin: "unknown" as const, originSource: "unconfirmed" as const, dockerLabels: { "com.docker.compose.service": "api" } };
  renderWithApp(<ContainerProvenance c={c} />, adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Herkunft bestätigen" }));
  const dialog = await screen.findByRole("dialog");
  const trigger = within(dialog).getByRole("combobox");
  trigger.focus(); await userEvent.keyboard("{ArrowDown}");
  expect(screen.queryByRole("option", { name: "Run" }) === null).toBe(true);
  await userEvent.keyboard("{Escape}");
  expect(within(dialog).getByText(/Unvollständige Compose-Labels/) !== null).toBe(true);
  await userEvent.click(within(dialog).getByRole("button", { name: "Herkunft bestätigen" }));
  await waitFor(() => expect(assign).toHaveBeenCalledExactlyOnceWith(c.id, "unraid"));
});
