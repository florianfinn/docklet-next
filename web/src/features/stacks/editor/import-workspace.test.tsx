// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original import Apply confirmation proof in the real new-stack workspace. M19 web consumer integration.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";
import StackPage from "../detail/StackPage";
beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });
it("edited new stack asks once before replacing compose and never deploys on Apply", async () => {
  const adapter = createDemoAdapter();
  adapter.importSource = async () => ({ title: "proposal-app", composeContent: "services:\n  imported:\n    image: example/app:1\n", envNames: ["TOKEN"], found: ["import.found.compose"], missing: [] });
  const deploy = vi.spyOn(adapter, "deployStack");
  renderRoutes([{ path: "/stacks/new", element: <StackPage /> }], ["/stacks/new"], adapter);
  const name = await screen.findByLabelText(/^Stackname/);
  await userEvent.clear(name); await userEvent.type(name, "my-stack");
  await userEvent.click(screen.getByRole("button", { name: /GitHub \/ Docker Hub/ }));
  await userEvent.type(screen.getByLabelText("Quelle"), "example/app"); await userEvent.click(screen.getByRole("button", { name: "Quelle auswerten" }));
  await screen.findByText("proposal-app"); await userEvent.click(screen.getByRole("button", { name: "Übernehmen" }));
  expect(screen.queryByRole("alertdialog") !== null).toBe(true);
  const confirm = screen.getByRole("alertdialog"); expect(within(confirm).getByRole("heading").textContent).toBe("Bearbeitete Compose-Datei ersetzen?");
  await userEvent.click(within(confirm).getByRole("button", { name: "Übernehmen" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog") === null).toBe(true));
  expect(deploy).toHaveBeenCalledTimes(0);
  expect(screen.queryByText("imported") !== null).toBe(true);
});
