// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regressions for rename, move and delete completion after view changes (M16 reviews CFR1-2, CFR2-1).

import { EditorView } from "@codemirror/view";
import { useIsFetching } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { revisionProducer } from "@/data/http/volume-revision.test-support";
import type { Volume } from "@/domain/volumes";
import { mockWideWorkspace } from "@/test/layout";
import { renderRoutes } from "@/test/render";

import { FileWorkspace } from "./FileWorkspace";

const volume: Volume = { id: "edge~data", hostId: "edge", name: "data", createdAt: "2026-10-22T12:00:00Z", kind: "volume", sizeBytes: 0, consumers: [], source: "", labels: {} };

function Page() {
  const fetching = useIsFetching({ queryKey: ["files"] });
  return <><output data-testid="fetching">{fetching}</output><FileWorkspace volume={volume} /></>;
}

beforeEach(() => { simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); mockWideWorkspace(); });
afterEach(() => { simulationStore.reset(); localStorage.clear(); vi.restoreAllMocks(); });

it.each([false, true])("keeps a dirty tab on the moved file and saves it there (tab switched while pending: %s)", async (switched) => {
  const producer = revisionProducer(true);
  producer.folders.add("dest");
  let release!: () => void;
  let started = false;
  const hold = new Promise<void>((resolve) => { release = resolve; });
  const moveEntry = vi.fn(async (...args: Parameters<typeof producer.workspace.moveEntry>) => { started = true; if (switched) await hold; return producer.workspace.moveEntry(...args); });
  const writeFile = vi.fn(producer.workspace.writeFile);
  const adapter = { ...createDemoAdapter(), ...producer.workspace, writeFile, moveEntry, kind: "http" as const, getCapability: async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: true }) };
  renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  const view = EditorView.findFromDOM(await screen.findByRole("textbox", { name: "Inhalt von a.txt" }))!;
  act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: "unsaved draft" } }));
  await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Aktionen für a.txt" }));
  await userEvent.click(screen.getByRole("menuitem", { name: "Verschieben" }));
  const folder = await screen.findByRole("textbox", { name: "Zielordner" });
  await userEvent.clear(folder);
  await userEvent.type(folder, "/dest");
  await userEvent.click(screen.getByRole("button", { name: "Verschieben" }));
  await waitFor(() => expect(started).toBe(true));
  if (switched) {
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Schließen" }));
    await userEvent.click(screen.getByRole("tab", { name: /^a.txt/ }));
    await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
    await act(async () => { release(); });
  }
  await waitFor(() => expect(producer.contents.has("dest/a.txt")).toBe(true));
  if (!switched) {
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    await userEvent.click(screen.getByRole("tab", { name: /^a.txt/ }));
  }
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await waitFor(() => expect(screen.getByTestId("fetching").textContent).toBe("0"));
  await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(false));
  expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("unsaved draft");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(writeFile.mock.calls.length).toBe(1));
  expect(writeFile.mock.calls.map((call) => call[1])).toEqual(["/dest/a.txt"]);
  await waitFor(() => expect(producer.contents.get("dest/a.txt")).toBe("unsaved draft"));
  expect([producer.contents.has("a.txt"), producer.conflicts]).toEqual([false, []]);
});

type Held = "move" | "rename" | "delete";

// A held command completes after the user opened another folder and reselected the file tab; completion reconciles the
// entry but keeps the user's folder.
it.each(["move", "rename", "delete"] as const)("keeps the newer folder when a held %s completes", async (operation: Held) => {
  const producer = revisionProducer(true);
  producer.folders.add("dest");
  let release!: () => void;
  let settled = false;
  const hold = new Promise<void>((resolve) => { release = resolve; });
  const held = <T,>(run: () => Promise<T>) => async () => { await hold; try { return await run(); } finally { settled = true; } };
  const adapter = {
    ...createDemoAdapter(), ...producer.workspace, kind: "http" as const,
    moveEntry: (...args: Parameters<typeof producer.workspace.moveEntry>) => held(() => producer.workspace.moveEntry(...args))(),
    renameEntry: (...args: Parameters<typeof producer.workspace.renameEntry>) => held(() => producer.workspace.renameEntry(...args))(),
    deleteEntry: (...args: Parameters<typeof producer.workspace.deleteEntry>) => held(() => producer.workspace.deleteEntry(...args))(),
    getCapability: async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: true }),
  };
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&path=%2F&file=%2Fa.txt"], adapter);
  const view = EditorView.findFromDOM(await screen.findByRole("textbox", { name: "Inhalt von a.txt" }))!;
  act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: "unsaved draft" } }));
  await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Aktionen für a.txt" }));
  const label = { move: "Verschieben", rename: "Umbenennen", delete: "Löschen" }[operation];
  await userEvent.click(screen.getByRole("menuitem", { name: label }));
  if (operation !== "delete") {
    const input = await screen.findByRole("textbox", { name: operation === "move" ? "Zielordner" : "Name" });
    await userEvent.clear(input);
    await userEvent.type(input, operation === "move" ? "/dest" : "renamed.txt");
  }
  await userEvent.click(within(await screen.findByRole(operation === "delete" ? "alertdialog" : "dialog")).getByRole("button", { name: label }));
  if (operation === "delete") {
    // The confirmation stays open while it runs; history navigation (back/forward) still changes the view.
    await act(async () => { await router.navigate("/volumes/edge~data?tab=files&path=%2Fdest&file=%2Fa.txt"); });
  } else {
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    await userEvent.click(screen.getByRole("button", { name: "dest öffnen" }));
    await userEvent.click(screen.getByRole("tab", { name: /^a.txt/ }));
  }
  await act(async () => { release(); });
  await waitFor(() => expect(settled).toBe(true));
  await waitFor(() => expect(screen.getByTestId("fetching").textContent).toBe("0"));
  const params = new URLSearchParams(router.state.location.search);
  const tabs = screen.getAllByRole("tab").map((tab) => tab.textContent ?? "");
  const expected = { move: "/dest/a.txt", rename: "/renamed.txt", delete: null }[operation];
  expect([params.get("path"), params.get("file"), tabs.some((label) => label.startsWith("a.txt") || label.startsWith("renamed.txt"))]).toEqual(["/dest", expected, operation !== "delete"]);
});
