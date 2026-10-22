// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original live workspace lifecycle regressions for issue #182.

import { useIsFetching } from "@tanstack/react-query";
import { EditorView } from "@codemirror/view";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useParams } from "react-router";
import type { DockletAdapter } from "@/data/adapter";
import type { VolumeFileContent } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { WorkspaceConflictError, WorkspaceError } from "@/data/http/workspace";
import type { Volume } from "@/domain/volumes";
import { renderRoutes } from "@/test/render";
import { FileWorkspace } from "./FileWorkspace";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); vi.restoreAllMocks(); });
const file = (path: string, content = path): VolumeFileContent => ({ path, content, editable: true, size: content.length, revision: "r1" });
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; }
function edit(text: string) { const view = EditorView.findFromDOM(screen.getByRole("textbox", { name: /^Inhalt von/ }))!; act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })); }
function Page() {
  const { id = "edge~data" } = useParams();
  const fetching = useIsFetching({ queryKey: ["files"] });
  const volume: Volume = { id, hostId: id.split("~")[0]!, name: "data", createdAt: "2026-10-22T12:00:00Z", kind: "volume", sizeBytes: 0, consumers: [], source: "", labels: {} };
  return <><output data-testid="file-reads">{fetching}</output><FileWorkspace volume={volume} /></>;
}
function fixture(overrides: Partial<DockletAdapter> = {}) {
  const listFiles = vi.fn(async (volumeId: string, path: string) => ({ volumeId, path, revision: "r1", entries: ["a.txt", "b.txt"].map((name) => ({ name, path: `/${name}`, editable: true, type: "file" as const, size: 3, modifiedAt: "2026-10-22T12:00:00Z" })) }));
  const adapter: DockletAdapter = { ...createDemoAdapter(), kind: "http", listFiles, readFile: vi.fn(async (_id, path) => file(path)), writeFile: vi.fn(async () => ({ revision: "r2" })), getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }), ...overrides };
  const rendered = renderRoutes([{ path: "/volumes/:id", element: <Page /> }, { path: "/elsewhere", element: <p>Elsewhere</p> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  return { ...rendered, adapter, listFiles };
}

describe("workspace lifecycle: late responses, unsaved navigation, conflicts and retry", () => {
  it("keeps the current file when an earlier file read arrives late", async () => {
    const late = deferred<VolumeFileContent>();
    const readFile = vi.fn(async (_id: string, path: string) => path === "/a.txt" ? late.promise : file(path, "current B"));
    const { router } = fixture({ readFile });
    await waitFor(() => expect(readFile).toHaveBeenCalledWith("edge~data", "/a.txt"));
    await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fb.txt"); });
    await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
    await act(async () => { late.resolve(file("/a.txt", "late A")); await late.promise; });
    await waitFor(() => expect(screen.getByTestId("file-reads").textContent).toBe("0"));
    expect(screen.getByRole("textbox", { name: "Inhalt von b.txt" }).textContent).toBe("current B");
  });
  it("isolates files and drafts when the volume and host change", async () => {
    const late = deferred<VolumeFileContent>();
    const readFile = vi.fn(async (id: string, path: string) => id === "edge~data" ? late.promise : file(path, "new host"));
    const { router } = fixture({ readFile });
    await waitFor(() => expect(readFile).toHaveBeenCalledWith("edge~data", "/a.txt"));
    await act(async () => { await router.navigate("/volumes/direct~other?tab=files&file=%2Fb.txt"); });
    await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
    await act(async () => { late.resolve(file("/a.txt", "late old host")); await late.promise; });
    await waitFor(() => expect(screen.getByTestId("file-reads").textContent).toBe("0"));
    expect(screen.queryByRole("tab", { name: /^a.txt/ }) === null).toBe(true);
    expect(screen.getByRole("textbox", { name: "Inhalt von b.txt" }).textContent).toBe("new host");
  });
  it("guards unsaved navigation to another host", async () => {
    const { router } = fixture(); await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); edit("draft");
    await act(async () => { void router.navigate("/volumes/direct~other?tab=files"); });
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect(router.state.location.pathname).toBe("/volumes/edge~data");
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("draft");
  });
  it("reloads the tree on conflict and keeps text until explicit discard", async () => {
    const writeFile = vi.fn(async () => { throw new WorkspaceConflictError(); });
    let reads = 0;
    const { listFiles } = fixture({ writeFile, readFile: async (_id, path) => file(path, ++reads === 1 ? "old" : "server new") });
    await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); edit("my draft");
    const count = listFiles.mock.calls.length;
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await screen.findByRole("alert");
    expect(listFiles.mock.calls.length).toBeGreaterThan(count);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("my draft");
    expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Änderungen verwerfen" }));
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("server new"));
  });
  it("recovers after a transient file read failure", async () => {
    const readFile = vi.fn().mockRejectedValueOnce(new WorkspaceError("unavailable")).mockResolvedValue(file("/a.txt", "recovered"));
    fixture({ readFile });
    await userEvent.click(await screen.findByRole("button", { name: "Erneut versuchen" }));
    expect((await screen.findByRole("textbox", { name: "Inhalt von a.txt" })).textContent).toBe("recovered");
  });
  it("coalesces pending saves and retains edits typed during the save", async () => {
    const pending = deferred<{ revision: string }>();
    const writeFile = vi.fn(() => pending.promise);
    fixture({ writeFile }); await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); edit("submitted");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(writeFile).toHaveBeenCalledTimes(1));
    edit("newer draft");
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Inhalt von a.txt" }), { key: "s", code: "KeyS", ctrlKey: true });
    await act(async () => { pending.resolve({ revision: "r2" }); await pending.promise; });
    expect(writeFile).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("newer draft");
    expect(screen.getByText("Ungespeicherte Änderungen", { selector: "[aria-live]" }) !== null).toBe(true);
  });
  it("uploads binary bytes and stops after a failed file without claiming later files", async () => {
    const first = deferred<void>();
    const uploadFile = vi.fn().mockImplementationOnce(() => first.promise).mockRejectedValueOnce(new WorkspaceError("too-large"));
    const { container } = fixture({ uploadFile });
    await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, { target: { files: [new File([new Uint8Array([0, 255])], "one.bin"), new File(["two"], "two.txt"), new File(["three"], "three.txt")] } });
    await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(1));
    await act(async () => { first.resolve(); await first.promise; });
    await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(2));
    expect((uploadFile.mock.calls[0]![2] as { blob: File }).blob.size).toBe(2);
    await waitFor(() => expect(input.value).toBe(""));
    expect(uploadFile).toHaveBeenCalledTimes(2);
  });
});

describe("live volume file gestures", () => {
  it("creates a file and folder, renames the file and deletes it", async () => {
    let names = ["a.txt", "b.txt"];
    const entries = () => names.map((name) => ({ name, path: `/${name}`, editable: true, type: name === "folder" ? "dir" as const : "file" as const, size: 3, modifiedAt: "2026-10-22T12:00:00Z" }));
    const createEntry = vi.fn(async (_id, _dir, name) => { names.push(name); });
    const renameEntry = vi.fn(async (_id, path, name) => { names = names.map((old) => `/${old}` === path ? name : old); });
    const deleteEntry = vi.fn(async (_id, path) => { names = names.filter((name) => `/${name}` !== path); });
    fixture({ createEntry, renameEntry, deleteEntry, listFiles: async (volumeId, path) => ({ volumeId, path, revision: "r1", entries: entries() }) });
    await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
    await userEvent.click(screen.getByRole("button", { name: "Neue Datei" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Name" }), "new.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
    await screen.findByRole("textbox", { name: "Inhalt von new.txt" });
    expect(createEntry).toHaveBeenCalledWith("edge~data", "/", "new.txt", "file", "r1");
    await userEvent.click(screen.getByRole("button", { name: "Neuer Ordner" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Name" }), "folder");
    await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Aktionen für new.txt" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Umbenennen" }));
    const input = within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Name" });
    await userEvent.clear(input); await userEvent.type(input, "renamed.txt");
    await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
    await userEvent.click(await screen.findByRole("button", { name: "Aktionen für renamed.txt" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Löschen" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Löschen" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Aktionen für renamed.txt" }) === null).toBe(true));
    expect(renameEntry).toHaveBeenCalledWith("edge~data", "/new.txt", "renamed.txt", "r1");
    expect(deleteEntry).toHaveBeenCalledWith("edge~data", "/renamed.txt", "r1");
  });
  it("downloads binary content through the download port without opening it as text", async () => {
    const blob = new Blob([new Uint8Array([0, 255])]); const downloadFile = vi.fn(async () => blob);
    const objectUrl = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:example");
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    fixture({ downloadFile, readFile: async (_id, path) => ({ ...file(path), editable: false, readOnlyReason: "binary", content: "" }) });
    await screen.findByText("Nicht als Text bearbeitbar");
    await userEvent.click(screen.getAllByRole("button", { name: "Herunterladen" })[0]!);
    await waitFor(() => expect(click).toHaveBeenCalledTimes(1));
    expect(downloadFile).toHaveBeenCalledWith("edge~data", "/a.txt");
    expect(objectUrl).toHaveBeenCalledWith(blob);
    expect(screen.queryByRole("textbox", { name: "Inhalt von a.txt" }) === null).toBe(true);
  });
});

describe("conflict recovery retains the user's draft", () => {
  for (const replacement of ["binary", "missing"] as const) {
    it(`keeps text visible when the server replacement is ${replacement}`, async () => {
      let reads = 0;
      fixture({
        writeFile: async () => { throw new WorkspaceConflictError(); },
        readFile: async (_id, path) => {
          if (++reads === 1) return file(path, "old");
          if (replacement === "missing") throw new WorkspaceError("not-found");
          return { ...file(path, ""), editable: false, readOnlyReason: "binary" };
        },
      });
      await screen.findByRole("textbox", { name: "Inhalt von a.txt" }); edit("my draft");
      await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
      await screen.findByRole("alert");
      await waitFor(() => expect(screen.getByTestId("file-reads").textContent).toBe("0"));
      expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("my draft");
      expect(screen.getByText("Ungespeicherte Änderungen", { selector: "[aria-live]" }) !== null).toBe(true);
    });
  }
});
