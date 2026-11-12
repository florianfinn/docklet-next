// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regressions for late completions while another workspace loads (M16 review CFR4-1).

import { EditorView } from "@codemirror/view";
import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useLayoutEffect } from "react";
import { useLocation, useParams } from "react-router";
import { toast } from "sonner";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { revisionProducer } from "@/data/http/volume-revision.test-support";
import type { Volume } from "@/domain/volumes";
import { mockWideWorkspace } from "@/test/layout";
import { renderRoutes } from "@/test/render";

import { FileWorkspace } from "@/features/volumes/files/FileWorkspace";

const volume: Volume = { id: "edge~data", hostId: "edge", name: "data", createdAt: "2026-10-22T12:00:00Z", kind: "volume", sizeBytes: 0, consumers: [], source: "", labels: {} };
let client: ReturnType<typeof useQueryClient>;
let committed = "";

function Page() {
  const { id } = useParams();
  const queryClient = useQueryClient();
  const location = useLocation();
  useLayoutEffect(() => { client = queryClient; committed = location.search; });
  const fetching = useIsFetching({ queryKey: ["files"] });
  return <><output data-testid="fetching">{fetching}</output><FileWorkspace volume={{...volume, id: id!}} /></>;
}

beforeEach(() => { simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); mockWideWorkspace(); });
afterEach(() => { simulationStore.reset(); localStorage.clear(); vi.restoreAllMocks(); });

type Operation = "move" | "rename" | "delete" | "create" | "upload" | "save";

function setup() {
  const producer = revisionProducer(true);
  producer.folders.add("dest");
  let release!: () => void;
  let started = false;
  const hold = new Promise<void>((resolve) => { release = resolve; });
  const held = <A extends unknown[], R>(run: (...args: A) => Promise<R>) => vi.fn(async (...args: A) => { started = true; await hold; return run(...args); });
  const methods = {
    moveEntry: held(producer.workspace.moveEntry), renameEntry: held(producer.workspace.renameEntry), deleteEntry: held(producer.workspace.deleteEntry),
    createEntry: held(producer.workspace.createEntry), uploadFile: held(producer.workspace.uploadFile), writeFile: held(producer.workspace.writeFile),
  };
  const adapter = { ...createDemoAdapter(), ...producer.workspace, ...methods, kind: "http" as const, getCapability: async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: true }) };
  let releaseLoad!: () => void;
  const loadGate = new Promise<void>(resolve => { releaseLoad = resolve; });
  const loader = async ({ request }: { request: Request }) => { if (new URL(request.url).searchParams.has("defer")) await loadGate; return null; };
  const rendered = renderRoutes([{ path: "/volumes/:id", element: <Page />, loader }, {path: "/outside", element: <p>Outside</p>, loader }], ["/volumes/edge~data?tab=files&path=%2F&file=%2Fa.txt"], adapter);
  return { producer, methods, release, releaseLoad, started: () => started, ...rendered };
}

async function start(operation: Operation, router: ReturnType<typeof setup>["router"], container: HTMLElement) {
  const editor = await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  if (operation === "create") {
    await userEvent.click(screen.getByRole("button", { name: "Neue Datei" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Name" }), "created.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
    return;
  }
  if (operation === "upload") {
    fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [new File(["first"], "one.txt"), new File(["second"], "two.txt")] } });
    return;
  }
  const view = EditorView.findFromDOM(editor)!;
  if (operation === "save") act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: "saved draft" } }));
  if (operation === "save") {
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    return;
  }
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
    await act(async () => { await router.navigate("/volumes/edge~data?tab=files&path=%2F&file=%2Fa.txt"); });
  } else {
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Schließen" }));
    await userEvent.click(screen.getByRole("tab", { name: /^a.txt/ }));
    await screen.findByRole("textbox", { name: "Inhalt von a.txt" });
  }
}

// Navigates when the command's refresh delivers revision r2, then lets the command complete before React commits that
// navigation. Commands that succeed visibly signal through their toast; create must not, so it waits for its settlement.
async function completeInsideWindow(f: ReturnType<typeof setup>, next: string, signal: "toast" | "settled", method: { mock: { settledResults: { type: string }[] } }) {
  const before = committed;
  let navigated = false;
  const off = client.getQueryCache().subscribe((event) => {
    if (!navigated && event.type === "updated" && event.action.type === "success" && event.query.queryKey[0] === "files" && event.query.queryKey[2] !== "content" && (event.query.state.data as { revision?: string })?.revision === "r2") {
      navigated = true;
      void f.router.navigate(next);
    }
  });
  let completed!: () => void;
  const completion = new Promise<void>((resolve) => { completed = resolve; });
  vi.spyOn(toast, "success").mockImplementation(() => { completed(); return "test"; });
  await act(async () => {
    f.release();
    if (signal === "toast") await completion;
    else {
      await vi.waitFor(() => expect(navigated && method.mock.settledResults[0]?.type === "fulfilled").toBe(true));
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    expect([navigated, committed === before]).toEqual([true, true]);
  });
  off();
  await waitFor(() => expect(screen.getByTestId("fetching").textContent).toBe("0"));
}

it.each([
 ["move","same"], ["rename","same"], ["delete","same"],
 ["move","volume"], ["rename","volume"], ["delete","volume"],
 ["create","same"], ["upload","same"], ["create","volume"], ["upload","volume"],
] as const)("%s completing during a loader preserves the %s workspace identity", async (operation, scenario) => {
 const f=setup();
 await start(operation, f.router, f.container);
 await waitFor(()=>expect(f.started()).toBe(true));
 const nextPath=scenario === "volume" ? "/volumes/edge~other" : "/volumes/edge~data";
 const next=nextPath+"?tab=files&path=%2F&file=%2Fa.txt&sort=name&defer=1#new";
 const method={move:f.methods.moveEntry,rename:f.methods.renameEntry,delete:f.methods.deleteEntry,create:f.methods.createEntry,upload:f.methods.uploadFile}[operation];
 await completeInsideWindow(f,next,operation === "create" ? "settled" : "toast",method);
 expect(f.router.state.navigation.state).toBe("loading");
 await act(async()=>{f.releaseLoad();});
 await waitFor(()=>expect(f.router.state.navigation.state).toBe("idle"));
 const params=new URLSearchParams(f.router.state.location.search);
 const expectedFile=scenario === "volume" ? "/a.txt" : {move:"/dest/a.txt",rename:"/renamed.txt",delete:null,create:"/created.txt",upload:"/a.txt"}[operation];
 expect([f.router.state.location.pathname,params.get("path"),params.get("file"),params.get("sort"),f.router.state.location.hash]).toEqual([nextPath,"/",expectedFile,"name","#new"]);
 if(operation === "upload") expect(f.methods.uploadFile.mock.calls.length).toBe(scenario === "same" ? 2 : 1);
});
