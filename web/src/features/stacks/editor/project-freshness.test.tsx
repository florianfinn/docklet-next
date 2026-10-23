// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 17796cbc61cdb65fd8ef7236ba301a2fe0286008
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original project tree refresh and older-draft regression matrix for issue #183.

import { useIsFetching } from "@tanstack/react-query";
import { useLayoutEffect } from "react";
import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { DockletAdapter } from "@/data/adapter";
import type { WorkspaceTree } from "@/data/api";
import { WorkspaceConflictError } from "@/data/http/workspace";
import { renderWithApp } from "@/test/render";
import { useProjectWorkspace, type ProjectWorkspace } from "./use-project-workspace";

let hook: ProjectWorkspace;
function Probe() { const value = useProjectWorkspace("edge~example", "compose"); useLayoutEffect(() => { hook = value; }, [value]); const fetching = useIsFetching(); return <><output data-testid="fetching">{fetching}</output><output data-testid="content">{value.files[0]?.draft?.text ?? value.files[0]?.data?.content}</output></>; }
afterEach(() => localStorage.clear());
function fixture() {
  let rev = 1, changed = false;
  let stored = "old";
  const folders = new Set<string>();
  const conflicts: string[] = [];
  const submitted: string[] = [];
  const tree = (): WorkspaceTree => ({ fileTreeRevision: `r${rev}`, fileTreeTruncated: false, files: [{ name: "a.txt", path: "/a.txt", relativePath: "a.txt", editable: true, isDirectory: false, isSymlink: false, size: stored.length, modTime: changed ? "2026-10-23T00:00:01Z" : "2026-10-23T00:00:00Z", mode: "-rw-r--r--" }, ...[...folders].map((name) => ({ name, relativePath: name, path: `/${name}`, editable: false, isDirectory: true, isSymlink: false, size: 0, modTime: "2026-10-23T00:00:00Z" }))] });
  const adapter: DockletAdapter = { ...createDemoAdapter(), kind: "http", getProjectWorkspace: async () => tree(), readProjectFile: async () => ({ name: "a.txt", path: "/a.txt", relativePath: "a.txt", content: stored, editable: true, size: stored.length, mimeType: "text/plain" }), updateProjectWorkspace: async (_id, revision, changes) => {
    submitted.push(revision);
    if (revision !== `r${rev}` || changes.some((change) => change.kind === "update" && change.baseline !== stored)) { conflicts.push(revision); throw new WorkspaceConflictError(); }
    for (const change of changes) if (change.kind === "update") stored = change.content;
    for (const change of changes) if (change.kind === "create-folder") folders.add(change.path);
    if (changes.some((change) => change.kind !== "update")) rev++; return tree();
  } };
  renderWithApp(<Probe />, adapter);
  return { conflicts, submitted, value: () => stored, externalContentOnly() { stored = "external"; changed = true; }, external(same: boolean) { rev++; folders.add("external-folder"); if (same) { stored = "external"; changed = true; } } };
}
async function ready() { await waitFor(() => expect(hook.tree.data?.fileTreeRevision).toBe("r1")); act(() => hook.open("a.txt")); await waitFor(() => expect(screen.getByTestId("content").textContent).toBe("old")); }

it.each([false, true])("project clean/dirty=%s adopts a tree reload after external changes elsewhere", async (dirty) => {
  const p = fixture(); await ready(); if (dirty) act(() => hook.edit("a.txt", "my draft"));
  p.external(false); await act(async () => { await hook.tree.refetch(); }); await waitFor(() => expect(hook.tree.data?.fileTreeRevision).toBe("r2"));
  act(() => hook.edit("a.txt", "my edit"));
  await act(async () => { await hook.save().catch(() => {}); });
  expect(p.conflicts).toEqual([]); expect(p.value()).toBe("my edit");
});
it.each([false, true])("project clean/dirty=%s handles external same-path content on reload", async (dirty) => {
  const p = fixture(); await ready(); if (dirty) act(() => hook.edit("a.txt", "my draft"));
  p.external(true); await act(async () => { await hook.tree.refetch(); }); await waitFor(() => expect(hook.tree.data?.fileTreeRevision).toBe("r2"));
  await waitFor(() => expect(screen.getByTestId("fetching").textContent).toBe("0"));
  if (!dirty) expect(screen.getByTestId("content").textContent).toBe("external");
  act(() => hook.edit("a.txt", "my edit"));
  await act(async () => { await hook.save().catch(() => {}); });
  expect(p.value()).toBe(dirty ? "external" : "my edit"); expect(p.conflicts.length).toBe(dirty ? 1 : 0);
});
it("MU11 an older changed project draft retains its revision through another own manifest", async () => {
  const p = fixture(); await ready(); act(() => hook.edit("a.txt", "my draft"));
  p.external(true); await act(async () => { await hook.tree.refetch(); }); await waitFor(() => expect(hook.tree.data?.fileTreeRevision).toBe("r2"));
  expect(hook.files[0]?.draft?.revision).toBe("r1");
  await act(async () => { await hook.gesture({ kind: "create-folder", path: "own" }); });
  expect(hook.tree.data?.fileTreeRevision).toBe("r3");
  expect(hook.files[0]?.draft?.revision).toBe("r1");
  expect(hook.files[0]?.draft?.saved).toBe("old");
  await act(async () => { await hook.save().catch(() => {}); });
  expect(p.submitted).toEqual(["r2", "r1"]); expect(p.value()).toBe("external");
});

it.each([false, true])("project content-only external edit keeps the structural revision (dirty=%s)", async (dirty) => {
  const p = fixture(); await ready(); if (dirty) act(() => hook.edit("a.txt", "my draft"));
  p.externalContentOnly(); await act(async () => { await hook.tree.refetch(); });
  await waitFor(() => expect(screen.getByTestId("fetching").textContent).toBe("0"));
  expect(hook.tree.data?.fileTreeRevision).toBe("r1");
  if (!dirty) expect(screen.getByTestId("content").textContent).toBe("external");
  act(() => hook.edit("a.txt", "my edit"));
  await act(async () => { await hook.save().catch(() => {}); });
  expect(p.submitted).toEqual(["r1"]);
  expect(p.conflicts).toEqual(dirty ? ["r1"] : []);
  expect(p.value()).toBe(dirty ? "external" : "my edit");
});
