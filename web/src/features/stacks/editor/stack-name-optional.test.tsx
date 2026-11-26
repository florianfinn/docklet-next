// Source: docklet-next/issues/652
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original coverage for an optional top-level Compose name on existing stacks.

import { EditorView } from "@codemirror/view";
import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { parseCompose } from "@/domain/compose/model";
import { validateCompose } from "@/domain/compose/validate";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const unnamed = "# upstream template without a top-level name\nservices:\n  app:\n    image: nginx:1.29-alpine\n";

describe("Compose name in the stack workspace (#652)", () => {
  it("accepts an existing stack without a top-level name and keeps redeploy enabled", async () => {
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    const editor = await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const view = EditorView.findFromDOM(editor)!;
    await act(async () => { view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: unnamed } }); });

    expect(screen.queryByText("Stackname fehlt") === null).toBe(true);
    expect(screen.getByText(/0 Fehler/) !== null).toBe(true);
    expect((screen.getByRole("button", { name: "Speichern & Redeploy" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("still requires a name for a new stack", () => {
    const parsed = parseCompose(unnamed);
    if (!parsed.ok) throw new Error("fixture must parse");
    expect(validateCompose(parsed.model).map((i) => i.code)).toEqual(["name-missing"]);
    expect(validateCompose(parsed.model, { existing: true }).map((i) => i.code)).toEqual([]);
  });
});
