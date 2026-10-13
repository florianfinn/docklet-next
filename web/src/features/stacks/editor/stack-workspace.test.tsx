import { EditorView } from "@codemirror/view";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const editorView = (label: string) => {
  const view = EditorView.findFromDOM(screen.getByRole("textbox", { name: label }));
  if (!view) throw new Error("no editor");
  return view;
};

describe("Compose workspace on the shared editor module", () => {
  it("shows tree, tabs, CodeMirror and mounted links to the volume workspace", async () => {
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    const editor = await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    expect(editor.closest('[data-slot="code-editor"]')?.getAttribute("data-language")).toBe("yaml");
    expect(editor.closest('[data-slot="workspace"]')?.querySelector('[data-slot="file-tree"]') !== null).toBe(true);

    const tree = screen.getByRole("navigation", { name: "Dateien" });
    const link = await within(tree).findByRole("link", { name: /^media_config/ });
    expect(link.getAttribute("href")).toBe("/volumes/vol-atlas-media_media_config?tab=files");

    await userEvent.click(screen.getByRole("tab", { name: /^\.env/ }));
    const env = await screen.findByRole("textbox", { name: "Inhalt von .env" });
    expect(env.closest('[data-slot="code-editor"]')?.getAttribute("data-language")).toBe("env");
    await userEvent.click(within(tree).getByRole("button", { name: /compose\.yaml/ }));
    expect(screen.getByRole("tab", { name: /^compose\.yaml/ }).getAttribute("aria-selected")).toBe("true");
  });

  it("keeps Easy and raw text in sync and locks Easy on invalid YAML", async () => {
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const modes = screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" });

    await userEvent.click(within(modes).getByRole("radio", { name: "Easy" }));
    await userEvent.click(within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getByRole("button", { name: /Stack/ }));
    const name = await screen.findByLabelText(/^Stackname/);
    await userEvent.clear(name);
    await userEvent.type(name, "medien");
    await userEvent.click(within(modes).getByRole("radio", { name: "Advanced" }));
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const doc = editorView("Inhalt von compose.yaml").state.doc.toString();
    // Targeted edit: the name changed, comments and unknown keys stayed.
    expect(/^name: "?medien"?$/m.test(doc) && doc.includes("# Media-Stack (synthetisch)") && doc.includes("x-docklet:")).toBe(true);
    expect(within(screen.getByRole("tab", { name: /^compose\.yaml/ })).queryByLabelText("Ungespeicherte Änderungen") !== null).toBe(true);

    const view = editorView("Inhalt von compose.yaml");
    act(() => view.dispatch({ changes: { from: view.state.doc.length, insert: "services: [\n" } }));
    await waitFor(() => expect(screen.getByText("Ungültig – nichts wird überschrieben") !== null).toBe(true));
    await userEvent.click(within(modes).getByRole("radio", { name: "Easy" }));
    expect(screen.getByText("Easy ist gesperrt, bis das YAML wieder gültig ist.") !== null).toBe(true);
  });
});
