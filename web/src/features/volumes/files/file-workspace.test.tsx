import { EditorView } from "@codemirror/view";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { mockWideWorkspace } from "@/test/layout";
import { renderRoutes } from "@/test/render";

import VolumePage from "../VolumePage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  mockWideWorkspace();
});

afterEach(() => {
  vi.restoreAllMocks();
  simulationStore.reset();
  localStorage.clear();
});

const HA = "vol-atlas-home_ha_config";
const PG = "vol-atlas-paperless_pg_data";

/** Inserts text through CodeMirror's own transaction API, as typing would. */
function appendText(label: string, text: string) {
  const content = screen.getByRole("textbox", { name: label });
  const view = EditorView.findFromDOM(content);
  if (!view) throw new Error("no editor");
  act(() => view.dispatch({ changes: { from: view.state.doc.length, insert: text } }));
}

const renderVolume = (url: string, adapter = createDemoAdapter()) =>
  renderRoutes(
    [
      { path: "/volumes/:volumeId", element: <VolumePage /> },
      { path: "/stacks/:stackId", element: <p>stack</p> },
    ],
    [url],
    adapter,
  );

describe("volume file workspace", () => {
  it("opens a file from the tree in a CodeMirror tab with the inferred language", async () => {
    const { router } = renderVolume(`/volumes/${HA}?tab=files`);
    const tree = await screen.findByRole("navigation", { name: "Dateien" });
    await userEvent.click(await within(tree).findByRole("button", { name: /configuration\.yaml/ }));
    const tab = await screen.findByRole("tab", { name: /^configuration\.yaml/ });
    expect(tab.getAttribute("aria-selected")).toBe("true");
    const editor = await screen.findByRole("textbox", { name: "Inhalt von configuration.yaml" });
    expect(editor.closest('[data-slot="code-editor"]')?.getAttribute("data-language")).toBe("yaml");
    const pane = editor.closest('[data-slot="editor-pane"]') as HTMLElement;
    expect(within(pane).getByText("YAML") !== null).toBe(true);
    // The details aside describes the open file next to the editor.
    const aside = document.querySelector('[data-slot="workspace-aside"]') as HTMLElement;
    expect(within(aside).getByText("/configuration.yaml") !== null && within(aside).getByText("home-homeassistant-1") !== null).toBe(true);
    expect(new URLSearchParams(router.state.location.search).get("file")).toBe("/configuration.yaml");
  });

  it("keeps unsaved edits across tabs, guards closing and leaving, and saves in memory", async () => {
    const adapter = createDemoAdapter();
    const { router } = renderVolume(`/volumes/${HA}?tab=files&file=%2Fconfiguration.yaml`, adapter);
    await screen.findByRole("textbox", { name: "Inhalt von configuration.yaml" });
    appendText("Inhalt von configuration.yaml", "logger:\n  default: info\n");
    const tab = screen.getByRole("tab", { name: /^configuration\.yaml/ });
    expect(within(tab).queryByLabelText("Ungespeicherte Änderungen") !== null).toBe(true);
    expect(screen.getByText("Ungespeicherte Änderungen", { selector: "[aria-live]" }) !== null).toBe(true);

    // Switching to the folder tab and back keeps the draft.
    await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
    await screen.findByRole("button", { name: "configuration.yaml öffnen" });
    await userEvent.click(screen.getByRole("tab", { name: /^configuration\.yaml/ }));
    expect((await screen.findByRole("textbox", { name: "Inhalt von configuration.yaml" })).textContent?.includes("default: info")).toBe(true);

    // Closing a dirty tab (Delete on the tab or its close mark) asks first; cancel keeps it.
    expect(screen.getByTitle("configuration.yaml schließen") instanceof HTMLElement).toBe(true);
    screen.getByRole("tab", { name: /^configuration\.yaml/ }).focus();
    await userEvent.keyboard("{Delete}");
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Abbrechen" }));
    expect(screen.getByRole("tab", { name: /^configuration\.yaml/ }) !== null).toBe(true);

    // Leaving the files tab is blocked while a file is dirty.
    await userEvent.click(screen.getByRole("tab", { name: /Übersicht/ }));
    const leave = await screen.findByRole("alertdialog");
    expect(within(leave).getByText(/configuration\.yaml/) !== null).toBe(true);
    await userEvent.click(within(leave).getByRole("button", { name: "Abbrechen" }));
    expect(new URLSearchParams(router.state.location.search).get("tab")).toBe("files");

    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByText("Gespeichert", { selector: "[aria-live]" }) !== null).toBe(true));
    expect((await adapter.readFile(HA, "/configuration.yaml")).content.endsWith("logger:\n  default: info\n")).toBe(true);
    await userEvent.click(screen.getByRole("tab", { name: /Übersicht/ }));
    await waitFor(() => expect(new URLSearchParams(router.state.location.search).get("tab")).toBe("overview"));
  });

  it("walks folders through the tree and shows non-text files read-only", async () => {
    renderVolume(`/volumes/${PG}?tab=files`);
    const tree = await screen.findByRole("navigation", { name: "Dateien" });
    const base = await within(tree).findByRole("button", { name: /^base/ });
    expect(base.getAttribute("aria-expanded")).toBe("false");
    await userEvent.click(base);
    expect(base.getAttribute("aria-expanded")).toBe("true");
    await userEvent.click(await within(tree).findByRole("button", { name: /^16384/ }));
    await userEvent.click(await within(tree).findByRole("button", { name: /^2619/ }));
    expect((await screen.findAllByText("Nicht als Text bearbeitbar")).length > 0).toBe(true);
    expect(screen.queryByRole("textbox", { name: /^Inhalt von/ }) === null).toBe(true);
    expect(screen.getAllByText("Nur lesen").length > 0).toBe(true);

    // Keyboard: arrow keys move between tree rows.
    base.focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement?.textContent?.includes("1")).toBe(true);

    await userEvent.click(within(tree).getByRole("button", { name: /^postgresql\.conf/ }));
    const conf = await screen.findByRole("textbox", { name: "Inhalt von postgresql.conf" });
    expect(conf.closest('[data-slot="code-editor"]')?.getAttribute("data-language")).toBe("ini");
  });
});
