// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original project workspace fixtures derived from the workspace producer for issue #182.

import { EditorView } from "@codemirror/view";
import { act, screen } from "@testing-library/react";
import { useParams } from "react-router";
import { vi } from "vitest";
import type { DockletAdapter } from "@/data/adapter";
import type { WorkspaceContent, WorkspaceTree } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { Stack } from "@/domain/model";
import { renderRoutes } from "@/test/render";
import { StackWorkspace } from "./StackWorkspace";
import { useComposeDraft } from "./use-compose-draft";

export const tree: WorkspaceTree = {
  files: ["a.txt", "b.txt"].map((name) => ({ name, relativePath: name, path: `/projects/example/${name}`, editable: true, isDirectory: false, isSymlink: false, modTime: "2026-10-22T12:00:00Z", size: 3 })),
  fileTreeRevision: "r1", fileTreeTruncated: false,
};
export const content = (path: string, value = "old"): WorkspaceContent => ({ name: path, path: `/projects/example/${path}`, relativePath: path, content: value, editable: true, mimeType: "text/plain", size: value.length });
export function deferred<T>() { let resolve!: (value: T) => void; let reject!: (cause: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; }
export function replaceText(label: string, value: string) { const view = EditorView.findFromDOM(screen.getByRole("textbox", { name: label }))!; act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })); }
const compose = "name: example\nservices:\n  app:\n    image: nginx:1.29-alpine\n";
// eslint-disable-next-line react-refresh/only-export-components -- Test-only router fixture.
function Page() { const { id = "edge~example" } = useParams(); return <EditorPage key={id} id={id} />; }
// eslint-disable-next-line react-refresh/only-export-components -- Test-only editor fixture.
function EditorPage({ id }: { id: string }) {
  const draft = useComposeDraft({ compose, env: "", hostId: id.split("~")[0]!, labels: [] });
  const stack = { id, hostId: draft.hostId, name: "example", labels: [] } as unknown as Stack;
  return <StackWorkspace draft={draft} stack={stack} hosts={[]} labelSuggestions={[]} mode="advanced" onMode={() => {}} />;
}
export function projectFixture(overrides: Partial<DockletAdapter> = {}) {
  const order: string[] = [];
  const getProjectWorkspace = vi.fn(async () => tree);
  const updateProjectWorkspace = vi.fn(async () => { order.push("extra"); return { ...tree, fileTreeRevision: "r2" }; });
  const saveStackFiles = vi.fn(async () => { order.push("compose"); });
  const deployStack = vi.fn(async () => { order.push("deploy"); return { stackId: "edge~example", actionId: "run" }; });
  const adapter: DockletAdapter = { ...createDemoAdapter(), kind: "http", getProjectWorkspace, readProjectFile: vi.fn(async (_id, path) => content(path)), updateProjectWorkspace, saveStackFiles, deployStack, getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }), ...overrides };
  const rendered = renderRoutes([{ path: "/stacks/:id", element: <Page /> }, { path: "/elsewhere", element: <p>Elsewhere</p> }], ["/stacks/edge~example?tab=editor"], adapter);
  return { ...rendered, adapter, order, getProjectWorkspace, updateProjectWorkspace, saveStackFiles, deployStack };
}
