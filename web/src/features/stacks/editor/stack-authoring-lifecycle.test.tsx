// Source: docklet-next/docs/design/stack-authoring.md
// Revision: 6655813e22d3d562a0ba91d6c4cb63796538f0ff
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original regressions for M7 review findings M7-R3-01 and M7-R3-02, after the reviewer's probes.

import { EditorView } from "@codemirror/view";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StackCreatedError, type DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

beforeEach(() => { simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { simulationStore.reset(); localStorage.clear(); });

const routes = [{ path: "/stacks/new", element: <StackPage /> }, { path: "/stacks/:stackId", element: <StackPage /> }];
const allowed: DockletAdapter["getCapability"] = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
const asHttp = (overrides: Partial<DockletAdapter> = {}): DockletAdapter => ({ ...createDemoAdapter(), kind: "http", getCapability: allowed, ...overrides });

async function addLabel(text: string) {
  await userEvent.click(within(screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" })).getByRole("radio", { name: "Easy" }));
  await userEvent.click(within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getByRole("button", { name: /Stack/ }));
  await userEvent.type(await screen.findByPlaceholderText("z. B. produktiv"), text);
  await userEvent.click(screen.getByRole("button", { name: "Label hinzufügen" }));
}

describe("late deploy completion", () => {
  for (const outcome of ["success", "refused", "unknown"] as const) {
    it(`keeps a later edited stack after a new-stack ${outcome}`, async () => {
      let settle: (fail: boolean) => void = () => {};
      const deployStack = vi.fn(() => new Promise<{ actionId: string; stackId: string }>((resolve, reject) => {
        settle = (fail) => fail
          ? reject(new StackCreatedError("atlas~created", new AdapterOutcomeError({ kind: "error", code: outcome === "unknown" ? "runtime.admission-unknown" : "run.refused" }), outcome === "unknown" ? "unknown" : "refused"))
          : resolve({ actionId: "r1", stackId: "atlas~created" });
      }));
      const { router } = renderRoutes(routes, ["/stacks/new"], asHttp({ deployStack }));
      const deploy = await screen.findByRole("button", { name: "Deploy" });
      await waitFor(() => expect((deploy as HTMLButtonElement).disabled).toBe(false));
      await userEvent.click(deploy);
      await waitFor(() => expect(deployStack).toHaveBeenCalledTimes(1));
      await act(async () => { await router.navigate("/stacks/media?tab=editor"); });
      await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
      await addLabel("LaterDraft");
      await act(async () => { settle(outcome !== "success"); });
      expect(router.state.location.pathname).toBe("/stacks/media");
      expect(screen.queryByText("laterdraft") !== null).toBe(true);
    });
  }
});

describe("keyboard save", () => {
  for (const denied of ["saveStackFiles", "setStackLabels"]) {
    it(`observes an unavailable ${denied} descriptor`, async () => {
      const saveStackFiles = vi.fn(async () => {}), setStackLabels = vi.fn(async () => {});
      const getCapability: DockletAdapter["getCapability"] = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: operationId !== denied, disabledReasonCode: "capability.unavailable" });
      renderRoutes(routes, ["/stacks/media?tab=editor"], asHttp({ saveStackFiles, setStackLabels, getCapability }));
      let editor = await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
      if (denied === "setStackLabels") {
        await addLabel("ExampleLabel");
        await userEvent.click(within(screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" })).getByRole("radio", { name: "Advanced" }));
        editor = await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
      } else {
        const view = EditorView.findFromDOM(editor)!;
        await act(async () => { view.dispatch({ changes: { from: view.state.doc.length, insert: "\n# example" } }); });
      }
      await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true));
      // Dispatches CodeMirror's own Mod-s binding.
      await act(async () => { fireEvent.keyDown(editor, { key: "s", code: "KeyS", ctrlKey: true }); });
      expect(saveStackFiles).toHaveBeenCalledTimes(0);
    });
  }
});
