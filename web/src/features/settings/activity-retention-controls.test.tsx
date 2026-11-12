// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original retention-field validation and read-only capability proofs. M19 web consumer integration.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";
import { ActivitySection } from "./sections/SimpleSections";
beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());
it("history fields validate ranges and save configured counts", async () => {
  const adapter = createDemoAdapter(), settings = await adapter.getSettings(), save = vi.spyOn(adapter, "saveSettings");
  renderWithApp(<ActivitySection settings={settings} />, adapter);
  const actions = screen.getByLabelText("Höchstens abgeschlossene Aktionen");
  await waitFor(() => expect((actions as HTMLInputElement).disabled).toBe(false));
  await userEvent.clear(actions); await userEvent.type(actions, "1001");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  expect(save).toHaveBeenCalledTimes(0);
  await userEvent.clear(actions); await userEvent.type(actions, "10"); await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1)); expect(save.mock.calls[0]?.[1]).toMatchObject({ maxFinishedActions: 10 });
});
it("UI configuration lock makes both retention fields read-only", async () => {
  const adapter = createDemoAdapter(), settings = await adapter.getSettings();
  adapter.getCapability = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: false, disabledReasonCode: "settings.ui-configuration-disabled" });
  renderWithApp(<ActivitySection settings={settings} />, adapter);
  await screen.findByText("Einstellungen dieses Hosts werden über seine Umgebung verwaltet");
  expect((screen.getByLabelText("Höchstens Ereignisse") as HTMLInputElement).disabled).toBe(true);
  expect((screen.getByLabelText("Höchstens abgeschlossene Aktionen") as HTMLInputElement).disabled).toBe(true);
});
