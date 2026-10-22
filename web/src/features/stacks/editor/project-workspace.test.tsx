// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original ordered save and multi-file workspace regressions for issue #182.

import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { simulationStore } from "@/data/demo/simulation";
import { WorkspaceConflictError, WorkspaceError } from "@/data/http/workspace";
import type { WorkspaceContent, WorkspaceTree } from "@/data/api";
import { EditorView } from "@codemirror/view";
import { useIsFetching } from "@tanstack/react-query";
import { useState } from "react";
import { useParams } from "react-router";
import type { DockletAdapter } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { Stack } from "@/domain/model";
import { renderRoutes } from "@/test/render";
import { StackWorkspace } from "./StackWorkspace";
import { useComposeDraft } from "./use-compose-draft";

const tree: WorkspaceTree = {
  files: ["a.txt", "b.txt"].map((name) => ({ name, relativePath: name, path: `/projects/example/${name}`, editable: true, isDirectory: false, isSymlink: false, modTime: "2026-10-22T12:00:00Z", size: 3 })),
  fileTreeRevision: "r1", fileTreeTruncated: false,
};
const content = (path: string, value = "old"): WorkspaceContent => ({ name: path, path: `/projects/example/${path}`, relativePath: path, content: value, editable: true, mimeType: "text/plain", size: value.length });
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (cause: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; }
function replaceText(label: string, value: string) { const view = EditorView.findFromDOM(screen.getByRole("textbox", { name: label }))!; act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })); }
const compose = "name: example\nservices:\n  app:\n    image: nginx:1.29-alpine\n";
function Page() { const { id = "edge~example" } = useParams(); const fetching = useIsFetching({ queryKey: ["project-workspace-file"] }); return <><output data-testid="file-reads">{fetching}</output><EditorPage key={id} id={id} /></>; }
function EditorPage({ id }: { id: string }) {
  const [mode, setMode] = useState<"easy" | "advanced">("advanced");
  const draft = useComposeDraft({ compose, env: "", hostId: id.split("~")[0]!, labels: [] });
  const stack = { id, hostId: draft.hostId, name: "example", labels: [] } as unknown as Stack;
  return <StackWorkspace draft={draft} stack={stack} hosts={[]} labelSuggestions={[]} mode={mode} onMode={setMode} />;
}
function projectFixture(overrides: Partial<DockletAdapter> = {}) {
  const order: string[] = [];
  const getProjectWorkspace = vi.fn(async () => tree);
  const updateProjectWorkspace = vi.fn(async () => { order.push("extra"); return { ...tree, fileTreeRevision: "r2" }; });
  const saveStackFiles = vi.fn(async () => { order.push("compose"); });
  const deployStack = vi.fn(async () => { order.push("deploy"); return { stackId: "edge~example", actionId: "run" }; });
  const adapter: DockletAdapter = { ...createDemoAdapter(), kind: "http", getProjectWorkspace, readProjectFile: vi.fn(async (_id, path) => content(path)), updateProjectWorkspace, saveStackFiles, deployStack, getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }), ...overrides };
  const rendered = renderRoutes([{ path: "/stacks/:id", element: <Page /> }, { path: "/elsewhere", element: <p>Elsewhere</p> }], ["/stacks/edge~example?tab=editor"], adapter);
  return { ...rendered, adapter, order, getProjectWorkspace, updateProjectWorkspace, saveStackFiles, deployStack };
}

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });
async function openFile(name = "a.txt") {
  const nav = await screen.findByRole("navigation", { name: "Dateien" });
  await userEvent.click(await within(nav).findByRole("button", { name }));
  return screen.findByRole("textbox", { name: `Inhalt von ${name}` });
}
async function dirtyBoth() {
  const compose = await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
  replaceText(compose.getAttribute("aria-label")!, "name: example\nservices:\n  app:\n    image: nginx:1.29-alpine\n# changed\n");
  await openFile(); replaceText("Inhalt von a.txt", "extra draft");
}

describe("project multi-file save ordering", () => {
  it("saves dirty Compose before extra files, then deploys through the existing port", async () => {
    const { order, updateProjectWorkspace, deployStack } = projectFixture();
    await dirtyBoth();
    await userEvent.click(screen.getByRole("button", { name: "Speichern & Redeploy" }));
    await waitFor(() => expect(deployStack).toHaveBeenCalledTimes(1));
    expect(order).toEqual(["compose", "extra", "deploy"]);
    expect(updateProjectWorkspace).toHaveBeenCalledWith("edge~example", "r1", [{ kind: "update", path: "a.txt", content: "extra draft", baseline: "old" }]);
  });
  it("does not send extra files after a failed Compose save", async () => {
    const saveStackFiles = vi.fn(async () => { throw new WorkspaceError("unavailable"); });
    const { updateProjectWorkspace, deployStack } = projectFixture({ saveStackFiles });
    await dirtyBoth(); await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(false));
    expect(saveStackFiles).toHaveBeenCalledTimes(1);
    expect(updateProjectWorkspace).toHaveBeenCalledTimes(0);
    expect(deployStack).toHaveBeenCalledTimes(0);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("extra draft");
  });
  it("shows partial save and keeps drafts when extra files fail after Compose", async () => {
    const updateProjectWorkspace = vi.fn(async () => { throw new WorkspaceError("unavailable"); });
    const { saveStackFiles, deployStack } = projectFixture({ updateProjectWorkspace });
    await dirtyBoth(); await userEvent.click(screen.getByRole("button", { name: "Speichern & Redeploy" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Compose und .env wurden gespeichert");
    expect(saveStackFiles).toHaveBeenCalledTimes(1);
    expect(updateProjectWorkspace).toHaveBeenCalledTimes(1);
    expect(deployStack).toHaveBeenCalledTimes(0);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("extra draft");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Dateien" })).getByRole("button", { name: /^compose.yaml/ }));
    expect(screen.getByRole("textbox", { name: "Inhalt von compose.yaml" }).textContent).toContain("# changed");
  });
  it("saves an extra file without writing unchanged Compose", async () => {
    const { updateProjectWorkspace, saveStackFiles } = projectFixture();
    await openFile(); replaceText("Inhalt von a.txt", "extra only");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true));
    expect(updateProjectWorkspace).toHaveBeenCalledTimes(1); expect(saveStackFiles).toHaveBeenCalledTimes(0);
  });
});

describe("project workspace lifecycle", () => {
  it("ignores late file responses after switching files", async () => {
    const late = deferred<WorkspaceContent>();
    const readProjectFile = vi.fn(async (_id: string, path: string) => path === "a.txt" ? late.promise : content(path, "current B"));
    projectFixture({ readProjectFile });
    const nav = await screen.findByRole("navigation", { name: "Dateien" });
    await userEvent.click(await within(nav).findByRole("button", { name: "a.txt" }));
    await waitFor(() => expect(readProjectFile).toHaveBeenCalledWith("edge~example", "a.txt"));
    await openFile("b.txt");
    await act(async () => { late.resolve(content("a.txt", "late A")); await late.promise; });
    await waitFor(() => expect(screen.getByTestId("file-reads").textContent).toBe("0"));
    expect(screen.getByRole("textbox", { name: "Inhalt von b.txt" }).textContent).toBe("current B");
  });
  it("ignores late responses after changing stack and host", async () => {
    const late = deferred<WorkspaceContent>();
    const readProjectFile = vi.fn(async (id: string, path: string) => id === "edge~example" ? late.promise : content(path, "new host"));
    const { router } = projectFixture({ readProjectFile });
    const nav = await screen.findByRole("navigation", { name: "Dateien" }); await userEvent.click(await within(nav).findByRole("button", { name: "a.txt" }));
    await waitFor(() => expect(readProjectFile).toHaveBeenCalledTimes(1));
    await act(async () => { await router.navigate("/stacks/direct~other?tab=editor"); });
    await openFile("b.txt");
    await act(async () => { late.resolve(content("a.txt", "old host")); await late.promise; });
    await waitFor(() => expect(screen.getByTestId("file-reads").textContent).toBe("0"));
    expect(screen.queryByRole("tab", { name: "a.txt" }) === null).toBe(true);
    expect(screen.getByRole("textbox", { name: "Inhalt von b.txt" }).textContent).toBe("new host");
  });
  it("guards navigation with unsaved extra files", async () => {
    const { router } = projectFixture(); await openFile(); replaceText("Inhalt von a.txt", "draft");
    await act(async () => { void router.navigate("/elsewhere"); });
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Abbrechen" }));
    expect(router.state.location.pathname).toBe("/stacks/edge~example");
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("draft");
  });
  it("reloads the tree and preserves the text on revision conflict", async () => {
    let reads = 0;
    const { getProjectWorkspace } = projectFixture({ readProjectFile: async (_id, path) => content(path, ++reads === 1 ? "old" : "server"), updateProjectWorkspace: async () => { throw new WorkspaceConflictError(); } });
    await openFile(); replaceText("Inhalt von a.txt", "draft");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await screen.findByRole("alert");
    expect(getProjectWorkspace.mock.calls.length).toBeGreaterThan(1);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("draft");
    await userEvent.click(screen.getByRole("button", { name: "Dateiänderungen verwerfen" }));
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("server"));
  });
  it("recovers from a transient read failure", async () => {
    const readProjectFile = vi.fn().mockRejectedValueOnce(new WorkspaceError("unavailable")).mockResolvedValue(content("a.txt", "recovered"));
    projectFixture({ readProjectFile });
    const nav = await screen.findByRole("navigation", { name: "Dateien" }); await userEvent.click(await within(nav).findByRole("button", { name: "a.txt" }));
    await userEvent.click(await screen.findByRole("button", { name: "Erneut versuchen" }));
    expect((await screen.findByRole("textbox", { name: "Inhalt von a.txt" })).textContent).toBe("recovered");
  });
  it("blocks repeated Save while pending and preserves edits typed meanwhile", async () => {
    const pending = deferred<typeof tree>(); const updateProjectWorkspace = vi.fn(() => pending.promise);
    projectFixture({ updateProjectWorkspace }); await openFile(); replaceText("Inhalt von a.txt", "submitted");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(updateProjectWorkspace).toHaveBeenCalledTimes(1));
    replaceText("Inhalt von a.txt", "typed meanwhile");
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Inhalt von a.txt" }), { key: "s", code: "KeyS", ctrlKey: true });
    await act(async () => { pending.resolve({ ...tree, fileTreeRevision: "r2" }); await pending.promise; });
    expect(updateProjectWorkspace).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("typed meanwhile");
  });
});

describe("project tree gestures and read-only states", () => {
  it("creates, renames and deletes a project file through individual gestures", async () => {
    let snapshot = tree;
    const updateProjectWorkspace = vi.fn(async (_id, _revision, changes) => {
      const change = changes[0];
      if (change.kind === "create-file") snapshot = { ...tree, fileTreeRevision: "r2", files: [...tree.files!, { ...tree.files![0]!, name: "new.txt", relativePath: "new.txt" }] };
      if (change.kind === "rename") snapshot = { ...tree, fileTreeRevision: "r3", files: [...tree.files!, { ...tree.files![0]!, name: "renamed.txt", relativePath: "renamed.txt" }] };
      if (change.kind === "delete") snapshot = { ...tree, fileTreeRevision: "r4" };
      return snapshot;
    });
    projectFixture({ updateProjectWorkspace });
    const create = await screen.findByRole("button", { name: "Neue Datei" });
    await waitFor(() => expect((create as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(create);
    await userEvent.type(within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Relativer Pfad" }), "new.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anwenden" }));
    await screen.findByRole("textbox", { name: "Inhalt von new.txt" });
    await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
    const input = within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Neuer Name" });
    await userEvent.clear(input); await userEvent.type(input, "renamed.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anwenden" }));
    await openFile("renamed.txt");
    await userEvent.click(screen.getByRole("button", { name: "Löschen" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Löschen" }));
    await waitFor(() => expect(updateProjectWorkspace).toHaveBeenCalledTimes(3));
    expect(updateProjectWorkspace.mock.calls.map((call) => call[2])).toEqual([[{ kind: "create-file", path: "new.txt", content: "" }], [{ kind: "rename", path: "new.txt", name: "renamed.txt" }], [{ kind: "delete", path: "renamed.txt" }]]);
  });
  for (const readOnlyReason of ["binary", "too_large", "symlink", "special"] as const) {
    it(`keeps ${readOnlyReason} content out of the text editor`, async () => {
      projectFixture({ readProjectFile: async (_id, path) => ({ ...content(path), content: undefined, editable: false, readOnlyReason, size: 1e10 }) });
      const nav = await screen.findByRole("navigation", { name: "Dateien" });
      await userEvent.click(await within(nav).findByRole("button", { name: "a.txt" }));
      await screen.findByText("Schreibgeschützte Datei");
      expect(screen.queryByRole("textbox", { name: "Inhalt von a.txt" }) === null).toBe(true);
    });
  }
  it("keeps the tree unchanged if a multi-file upload fails", async () => {
    const settled = deferred<typeof tree>(); const updateProjectWorkspace = vi.fn(() => settled.promise);
    const { container } = projectFixture({ updateProjectWorkspace });
    await waitFor(() => expect((screen.getByRole("button", { name: "Dateien hochladen" }) as HTMLButtonElement).disabled).toBe(false));
    fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [new File([new Uint8Array([0, 255])], "one.bin"), new File(["two"], "two.txt")] } });
    await waitFor(() => expect(updateProjectWorkspace).toHaveBeenCalledTimes(1));
    await act(async () => { settled.reject(new WorkspaceError("too-large")); try { await settled.promise; } catch { /* Expected upload rejection. */ } });
    await waitFor(() => expect((screen.getByRole("button", { name: "Dateien hochladen" }) as HTMLButtonElement).disabled).toBe(false));
    expect(screen.queryByRole("tab", { name: "one.bin" }) === null).toBe(true);
    expect(screen.queryByRole("tab", { name: "two.txt" }) === null).toBe(true);
  });
});

describe("save and deploy snapshot ownership", () => {
  it("keeps newer text and defers deploy when it was typed during save", async () => {
    const pending = deferred<typeof tree>(); const updateProjectWorkspace = vi.fn(() => pending.promise);
    const { deployStack } = projectFixture({ updateProjectWorkspace });
    await openFile(); replaceText("Inhalt von a.txt", "submitted");
    await userEvent.click(screen.getByRole("button", { name: "Speichern & Redeploy" }));
    await waitFor(() => expect(updateProjectWorkspace).toHaveBeenCalledTimes(1));
    replaceText("Inhalt von a.txt", "typed meanwhile");
    await act(async () => { pending.resolve({ ...tree, fileTreeRevision: "r2" }); await pending.promise; });
    await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(false));
    expect(deployStack).toHaveBeenCalledTimes(0);
    expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("typed meanwhile");
  });
});

describe("late tree gesture completion", () => {
  it("keeps the current env tab when a dismissed create dialog completes", async () => {
    const pending = deferred<typeof tree>(); const updateProjectWorkspace = vi.fn(() => pending.promise);
    projectFixture({ updateProjectWorkspace });
    const create = await screen.findByRole("button", { name: "Neue Datei" });
    await waitFor(() => expect((create as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(create);
    await userEvent.type(within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Relativer Pfad" }), "new.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anwenden" }));
    await waitFor(() => expect(updateProjectWorkspace).toHaveBeenCalledTimes(1));
    await userEvent.keyboard("{Escape}");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Dateien" })).getByRole("button", { name: ".env" }));
    await screen.findByRole("textbox", { name: "Inhalt von .env" });
    await act(async () => { pending.resolve({ ...tree, fileTreeRevision: "r2" }); await pending.promise; });
    await waitFor(() => expect((screen.getByRole("button", { name: "Neue Datei" }) as HTMLButtonElement).disabled).toBe(false));
    expect(screen.getByRole("textbox", { name: "Inhalt von .env" }) !== null).toBe(true);
    expect(screen.queryByRole("tab", { name: "new.txt" }) === null).toBe(true);
  });
});

describe("project conflict replacement states", () => {
  for (const replacement of ["binary", "missing"] as const) {
    it(`keeps the draft visible after a ${replacement} replacement on the server`, async () => {
      let reads = 0;
      projectFixture({ updateProjectWorkspace: async () => { throw new WorkspaceConflictError(); }, readProjectFile: async (_id, path) => {
        if (++reads === 1) return content(path);
        if (replacement === "missing") throw new WorkspaceError("not-found");
        return { ...content(path), content: undefined, editable: false, readOnlyReason: "binary" };
      } });
      await openFile(); replaceText("Inhalt von a.txt", "my draft");
      await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
      await screen.findByRole("alert");
      await waitFor(() => expect(screen.getByTestId("file-reads").textContent).toBe("0"));
      expect(screen.getByRole("textbox", { name: "Inhalt von a.txt" }).textContent).toBe("my draft");
    });
  }
});

describe("file command target ownership", () => {
  it("targets the active project file after switching between open tabs", async () => {
    projectFixture();
    await openFile("a.txt");
    await openFile("b.txt");
    await userEvent.click(screen.getByRole("tab", { name: "a.txt" }));
    await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
    const input = within(await screen.findByRole("dialog")).getByRole("textbox", { name: "Neuer Name" }) as HTMLInputElement;
    expect(input.value).toBe("a.txt");
  });
});

describe("returning to the Compose editor", () => {
  it("opens Compose when fixing invalid YAML from Easy after an extra file", async () => {
    projectFixture();
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    replaceText("Inhalt von compose.yaml", "services: [");
    await openFile("a.txt");
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" })).getByRole("radio", { name: "Easy" }));
    await userEvent.click(screen.getByRole("button", { name: "In Advanced beheben" }));
    expect((await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" })).textContent).toBe("services: [");
  });
});
