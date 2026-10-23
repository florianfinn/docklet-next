// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4037e18b9c926793e2888952b4ec9c0805ecc62e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original whole-tree revision regression coverage for issue #183.

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EditorView } from "@codemirror/view";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useParams } from "react-router";
import type { DockletAdapter } from "@/data/adapter";
import type { VolumeFileContent, WorkspaceBaseline } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { revisionProducer } from "@/data/http/volume-revision.test-support";
import { WorkspaceConflictError } from "@/data/http/workspace";
import type { Host } from "@/domain/model";
import type { Volume } from "@/domain/volumes";
import { renderRoutes, renderWithApp } from "@/test/render";
import { FileWorkspace } from "./FileWorkspace";
import { useVolumeMenu } from "../volume-menu";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); vi.restoreAllMocks(); });
const volume: Volume = { id: "edge~data", hostId: "edge", name: "data", createdAt: "2026-10-22T12:00:00Z", kind: "volume", sizeBytes: 0, consumers: [], source: "", labels: {} };
function edit(name: string, text: string) { const view = EditorView.findFromDOM(screen.getByRole("textbox", { name: `Inhalt von ${name}` }))!; act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })); }
function Page() { useParams(); return <FileWorkspace volume={volume} />; }

// Producer semantics: one whole-tree revision; every successful write changes it.
it.each([false, true])("saving one tab preserves another open tab baseline (already drafted=%s)", async (drafted) => {
  let rev = 1;
  const conflicts: string[] = [];
  const adapter: DockletAdapter = {
    ...createDemoAdapter(), kind: "http",
    getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }),
    listFiles: async (volumeId, path) => ({ volumeId, path, revision: `r${rev}`, entries: ["a.txt", "b.txt"].map((name) => ({ name, path: `/${name}`, editable: true, type: "file" as const, size: 3, modifiedAt: "2026-10-22T12:00:00Z" })) }),
    readFile: async (_id, path): Promise<VolumeFileContent> => ({ path, content: path, editable: true, size: 3, revision: `r${rev}` }),
    writeFile: async (_id, path, _content, baseline?: WorkspaceBaseline) => {
      if (baseline?.revision !== `r${rev}`) { conflicts.push(`${path}@${baseline?.revision}≠r${rev}`); throw new WorkspaceConflictError(); }
      rev++; return { revision: `r${rev}` };
    },
  };
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fb.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
  if (drafted) edit("b.txt", "B draft");
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  edit("a.txt", "A edited");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(rev).toBe(2));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fb.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
  edit("b.txt", "B edited");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(rev > 2 || conflicts.length > 0).toBe(true));
  expect(conflicts).toEqual([]);
});

// Producer semantics from backend/internal/activity/capabilities.go: listFiles needs a volume ref.
it("the volume row menu enables Files with producer-shaped descriptors", async () => {
  const descriptors: string[] = [];
  const adapter: DockletAdapter = {
    ...createDemoAdapter(), kind: "http",
    getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => { descriptors.push(operationId); return { operationId, resourceRef, available: operationId !== "listFiles" || resourceRef.kind === "volume" }; },
  };
  const host = { id: "edge", status: "online" } as Host;
  function Probe() { const files = useVolumeMenu(volume, host, { remove: () => {}, backup: () => {} }).flat().find((a) => a.id === "files"); return <output data-testid="files">{String(files?.disabled)}</output>; }
  renderWithApp(<Probe />, adapter);
  await waitFor(() => expect(descriptors).toContain("listFiles"));
  await waitFor(() => expect(screen.getByTestId("files").textContent).toBe("false"));
});

it.each([false, true])("creating a folder preserves the open file baseline (already drafted=%s)", async (drafted) => {
  let rev = 1;
  const conflicts: string[] = [];
  const adapter: DockletAdapter = {
    ...createDemoAdapter(), kind: "http",
    getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }),
    listFiles: async (volumeId, path) => ({ volumeId, path, revision: `r${rev}`, entries: [{ name: "a.txt", path: "/a.txt", editable: true, type: "file" as const, size: 3, modifiedAt: "2026-10-22T12:00:00Z" }] }),
    readFile: async (_id, path): Promise<VolumeFileContent> => ({ path, content: path, editable: true, size: 3, revision: `r${rev}` }),
    createEntry: async (_id, _dir, _name, _type, revision) => { if (revision !== `r${rev}`) throw new WorkspaceConflictError(); rev++; return { revision: `r${rev}` }; },
    writeFile: async (_id, path, _content, baseline?: WorkspaceBaseline) => {
      if (baseline?.revision !== `r${rev}`) { conflicts.push(`${path}@${baseline?.revision}≠r${rev}`); throw new WorkspaceConflictError(); }
      rev++; return { revision: `r${rev}` };
    },
  };
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  if (drafted) edit("a.txt", "A draft");
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files"); });
  await userEvent.click(await screen.findByRole("button", { name: "Neuer Ordner" }));
  await userEvent.type(await screen.findByRole("textbox", { name: "Name" }), "folder");
  await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
  await waitFor(() => expect(rev).toBe(2));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  edit("a.txt", "A edited");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(rev > 2 || conflicts.length > 0).toBe(true));
  expect(conflicts).toEqual([]);
});

it("recovers an external conflict through the real adapter and saves the next edit once", async () => {
  const producer = revisionProducer();
  const adapter = { ...createDemoAdapter(), ...producer.workspace, kind: "http" as const, getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }) };
  renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  edit("a.txt", "my draft");
  producer.externalWrite();
  let release!: () => void;
  let started!: () => void;
  const pending = new Promise<void>((resolve) => { release = resolve; });
  const reached = new Promise<void>((resolve) => { started = resolve; });
  producer.hooks.tree = async () => { started(); await pending; };
  const reads = producer.events.filter((event) => event.startsWith("file:")).length;
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await act(async () => { await reached; });
  // Recovery must finish its listing refresh before it re-reads the content.
  expect(producer.events.filter((event) => event.startsWith("file:"))).toHaveLength(reads);
  expect((screen.getByRole("button", { name: "Änderungen verwerfen" }) as HTMLButtonElement).disabled).toBe(true);
  await act(async () => { producer.hooks.tree = undefined; release(); });
  await screen.findByRole("alert");
  await waitFor(() => expect((screen.getByRole("button", { name: "Änderungen verwerfen" }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(screen.getByRole("button", { name: "Änderungen verwerfen" }));
  await waitFor(() => expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("external"));
  edit("a.txt", "recovered edit");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true));
  expect(producer.contents.get("a.txt")).toBe("recovered edit");
  expect(producer.events.filter((event) => event.startsWith("put:"))).toEqual(["put:r1", "put:r2"]);
  expect(producer.conflicts).toEqual(["r1"]);
});

it("refreshes a clean tab after an external edit following an own manifest", async () => {
  const producer = revisionProducer();
  const adapter = { ...createDemoAdapter(), ...producer.workspace, kind: "http" as const, getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }) };
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fb.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
  edit("b.txt", "submitted B");
  producer.hooks.updated = () => { producer.hooks.updated = undefined; producer.externalWrite("a.txt", "external A"); };
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(producer.revision()).toBe("r3"));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await waitFor(() => expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("external A"));
  edit("a.txt", "my A");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(producer.contents.get("a.txt")).toBe("my A"));
  expect(producer.conflicts).toEqual([]);
});


it("does not advance an older baseline past an external edit before another tab saves", async () => {
  const producer = revisionProducer();
  const adapter = { ...createDemoAdapter(), ...producer.workspace, kind: "http" as const, getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }) };
  const { router } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  edit("a.txt", "my A");
  producer.externalWrite("a.txt", "external A");
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fb.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von b.txt" });
  edit("b.txt", "my B");
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(producer.revision()).toBe("r3"));
  await act(async () => { await router.navigate("/volumes/edge~data?tab=files&file=%2Fa.txt"); });
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await screen.findByRole("alert");
  expect(producer.conflicts).toEqual(["r1"]);
  expect(producer.contents.get("a.txt")).toBe("external A");
});

it.each(["save", "gesture"] as const)("serialises saves and gestures while an own %s is pending", async (pendingKind) => {
  const producer = revisionProducer();
  let release!: () => void;
  const pending = new Promise<void>((resolve) => { release = resolve; });
  const adapter: DockletAdapter = {
    ...createDemoAdapter(), ...producer.workspace, kind: "http",
    getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }),
    writeFile: vi.fn(async (...args: Parameters<DockletAdapter["writeFile"]>) => { if (pendingKind === "save") await pending; return producer.workspace.writeFile(...args); }),
    createEntry: vi.fn(async (...args: Parameters<DockletAdapter["createEntry"]>) => { if (pendingKind === "gesture") await pending; return producer.workspace.createEntry(...args); }),
    uploadFile: vi.fn(producer.workspace.uploadFile),
  };
  const { container } = renderRoutes([{ path: "/volumes/:id", element: <Page /> }], ["/volumes/edge~data?tab=files&file=%2Fa.txt"], adapter);
  await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  edit("a.txt", "my A");
  try {
    if (pendingKind === "save") {
      await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
      await waitFor(() => expect(adapter.writeFile).toHaveBeenCalledOnce());
      expect((screen.getByRole("button", { name: "Neuer Ordner" }) as HTMLButtonElement).disabled).toBe(true);
      fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [new File(["file"], "other.txt")] } });
      expect(adapter.uploadFile).not.toHaveBeenCalled();
    } else {
      await userEvent.click(screen.getByRole("button", { name: "Neuer Ordner" }));
      await userEvent.type(await screen.findByRole("textbox", { name: "Name" }), "folder");
      await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
      await waitFor(() => expect(adapter.createEntry).toHaveBeenCalledOnce());
      await userEvent.keyboard("{Escape}");
      expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true);
      fireEvent.keyDown(screen.getByRole("textbox", { name: "Inhalt von a.txt" }), { key: "s", code: "KeyS", ctrlKey: true });
      expect(adapter.writeFile).not.toHaveBeenCalled();
    }
  } finally { await act(async () => { release(); await pending; }); }
  if (pendingKind === "gesture") {
    await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  }
  await waitFor(() => expect(producer.contents.get("a.txt")).toBe("my A"));
  expect(producer.conflicts).toEqual([]);
});
