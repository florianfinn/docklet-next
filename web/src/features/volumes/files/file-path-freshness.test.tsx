// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 17796cbc61cdb65fd8ef7236ba301a2fe0286008
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Permanent P5/P6 transport regressions derived from the independent R2 probes.
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EditorView } from "@codemirror/view";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import { revisionProducer } from "@/data/http/volume-revision.test-support";
import { useParams } from "react-router";
import type { DockletAdapter } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { Volume } from "@/domain/volumes";
import { renderRoutes } from "@/test/render";
import { FileWorkspace } from "./FileWorkspace";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); vi.restoreAllMocks(); });
const volume: Volume = { id: "edge~data", hostId: "edge", name: "data", createdAt: "2026-10-22T12:00:00Z", kind: "volume", sizeBytes: 0, consumers: [], source: "", labels: {} };
function edit(name: string, text: string) { const view = EditorView.findFromDOM(screen.getByRole("textbox", { name: `Inhalt von ${name}` }))!; act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })); }
function Page() { useParams(); const client = useQueryClient(); const fetching = useIsFetching(); return <><button onClick={() => void client.invalidateQueries({ queryKey: ["files", volume.id], predicate: (q) => q.queryKey[2] !== "content" })}>Reload tree</button><output data-testid="fetching">{fetching}</output><FileWorkspace volume={volume} /></>; }
async function settled() { await waitFor(() => expect(screen.getByTestId("fetching").textContent).toBe("0")); }

function producer(compare = false) {
  const p = revisionProducer(compare);
  const adapter: DockletAdapter = { ...createDemoAdapter(), ...p.workspace, kind: "http", getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }) };
  return { ...p, adapter, rev: () => Number(p.revision().slice(1)) };
}

// P5: replace a file by delete + upload (create_file cannot overwrite), then reopen it.
it.each([false, true].flatMap((guard) => ["upload", "empty"].map((replacement) => [guard, replacement] as const)))("P5 reopening a file replaced by own delete+create shows current content (baseline guard=%s, replacement=%s)", async (guard, replacement) => {
  const p = producer(guard);
  const { router, container } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], p.adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await waitFor(() => expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("old A"));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files"); });
  await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Aktionen für a.txt" }));
  await userEvent.click(screen.getByRole("menuitem", { name: "Löschen" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Löschen" }));
  await waitFor(() => expect(p.rev()).toBe(2));
  await waitFor(() => expect(screen.queryByRole("button", { name: "Aktionen für a.txt" }) === null).toBe(true));
  const replacementContent = replacement === "upload" ? "uploaded A" : "";
  if (replacement === "upload") {
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File([replacementContent], "a.txt")] } });
  } else {
    await userEvent.click(screen.getByRole("button", { name: "Neue Datei" }));
    await userEvent.type(await screen.findByRole("textbox", { name: "Name" }), "a.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
  }
  await waitFor(() => expect(p.rev()).toBe(3));
  await waitFor(() => expect(p.contents.get("a.txt")).toBe(replacementContent));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await settled();
  const shown = screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent;
  edit("a.txt", "my edit");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(p.rev() > 3 || p.conflicts.length > 0).toBe(true));
  // Either the tab showed the uploaded content, or the stale baseline conflicted.
  // Data guard: a stale tab must never overwrite the uploaded content silently.
  expect({ stored: p.contents.get("a.txt"), conflicted: p.conflicts.length > 0 }).toEqual(
    shown === replacementContent ? { stored: "my edit", conflicted: false } : { stored: replacementContent, conflicted: true },
  );
  expect(shown).toBe(replacementContent);
});

// P6: swap names by two own renames while both tabs stay open.
it.each([false, true])("P6 a tab moved onto a path that previously held another file shows its own content (baseline guard=%s)", async (guard) => {
  const p = producer(guard);
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], p.adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fb.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
  await waitFor(() => expect(screen.getByRole("textbox", { name: "Inhalt von b.txt" }).textContent).toBe("old B"));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files"); });
  await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
  for (const [from, to] of [["a.txt", "c.txt"], ["b.txt", "a.txt"]] as const) {
    await userEvent.click(await screen.findByRole("button", { name: `Aktionen für ${from}` }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Umbenennen" }));
    const name = within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Name" });
    await userEvent.clear(name); await userEvent.type(name, to);
    await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
    await screen.findByRole("button", { name: `Aktionen für ${to}` });
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  }
  expect(p.contents.get("a.txt")).toBe("old B");
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await settled();
  const shown = screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent;
  edit("a.txt", "edited from shown");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(p.rev() > 3 || p.conflicts.length > 0).toBe(true));
  // Data guard: content shown from another file's identity must not be saved silently.
  expect({ stored: p.contents.get("a.txt"), conflicted: p.conflicts.length > 0 }).toEqual(
    shown === "old B" ? { stored: "edited from shown", conflicted: false } : { stored: "old B", conflicted: true },
  );
  expect(shown).toBe("old B");
});

it("S3-01 refreshes an unchanged clean tab through gesture conflict and retry", async () => {
  const p = producer(true);
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], p.adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  edit("a.txt", "own saved"); await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(p.rev()).toBe(2)); await settled();
  p.externalFolder("external");
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files"); });
  await userEvent.click(await screen.findByRole("button", { name: "Neuer Ordner" }));
  await userEvent.type(await screen.findByRole("textbox", { name: "Name" }), "own-folder");
  await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
  await waitFor(() => expect(p.conflicts).toEqual(["r2"])); await settled();
  await waitFor(() => expect((screen.getByRole("button", { name: "Anlegen" }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
  await waitFor(() => expect(p.rev()).toBe(4)); await settled();
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); edit("a.txt", "next saved");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(p.rev() > 4 || p.conflicts.length > 1).toBe(true));
  expect(p.conflicts).toEqual(["r2"]); expect(p.contents.get("a.txt")).toBe("next saved");
});

it.each([false, true])("reload reconciles clean/dirty=%s tabs with external changes elsewhere", async (dirty) => {
  const p = producer(true);
  renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], p.adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); await settled();
  if (dirty) edit("a.txt", "my draft");
  p.externalWrite("b.txt", "external B");
  await userEvent.click(screen.getByRole("button", { name: "Reload tree" })); await settled();
  edit("a.txt", "my edit"); await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(p.rev() > 2 || p.conflicts.length > 0).toBe(true));
  expect(p.conflicts).toEqual([]); expect(p.contents.get("a.txt")).toBe("my edit");
});

it.each([false, true])("reload reconciles clean/dirty=%s tabs with external changes to their own path", async (dirty) => {
  const p = producer(true);
  renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], p.adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); await settled();
  if (dirty) edit("a.txt", "my draft");
  p.externalWrite("a.txt", "external A");
  await userEvent.click(screen.getByRole("button", { name: "Reload tree" })); await settled();
  if (!dirty) expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("external A");
  edit("a.txt", "my edit"); await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(p.rev() > 2 || p.conflicts.length > 0).toBe(true));
  expect(p.contents.get("a.txt")).toBe(dirty ? "external A" : "my edit");
  expect(p.conflicts.length).toBe(dirty ? 1 : 0);
});
