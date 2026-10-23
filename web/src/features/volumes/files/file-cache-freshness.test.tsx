// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 17796cbc61cdb65fd8ef7236ba301a2fe0286008
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original subtree identity and cache timestamp regression coverage for issue #183.

import { act, waitFor } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useLayoutEffect, useRef } from "react";
import { afterEach, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { revisionProducer } from "@/data/http/volume-revision.test-support";
import { useFiles } from "@/data/queries";
import { touchedPaths } from "@/data/workspace-freshness";
import type { VolumeFileContent } from "@/data/api";
import { renderWithApp } from "@/test/render";
import { useOpenFiles } from "./use-open-files";
import { useFileCommands } from "./use-file-commands";

let open: ReturnType<typeof useOpenFiles>;
let commands: ReturnType<typeof useFileCommands>;
let client: QueryClient;
let displayedRevision: string | undefined;
function Probe() {
  const queryClient = useQueryClient();
  const listing = useFiles("edge~data", "/");
  const files = useOpenFiles("edge~data", null);
  const actions = useFileCommands("edge~data", listing.data?.revision, files.rebase, useRef(false));
  useLayoutEffect(() => { client = queryClient; open = files; commands = actions; displayedRevision = listing.data?.revision; }, [queryClient, files, actions, listing.data?.revision]);
  return null;
}
afterEach(() => localStorage.clear());
function fixture(guard: boolean) {
  const p = revisionProducer(guard);
  const adapter = { ...createDemoAdapter(), ...p.workspace, kind: "http" as const };
  return { ...p, render: () => renderWithApp(<Probe />, adapter) };
}
it("rebasing unchanged open content preserves dataUpdatedAt", async () => {
  const p = fixture(true); p.render();
  act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  const key = ["files", "edge~data", "content", "/a.txt"];
  act(() => client.setQueryData(key, client.getQueryData(key), { updatedAt: 1234 }));
  act(() => open.rebase("r1", "r2", ["/folder"]));
  expect(client.getQueryData<VolumeFileContent>(key)?.revision).toBe("r2");
  expect(client.getQueryState(key)?.dataUpdatedAt).toBe(1234);
});
it.each([false, true])("folder move replaces a cached subtree identity (baseline guard=%s)", async (guard) => {
  const p = fixture(guard);
  p.contents.clear(); p.contents.set("left/cache/a.txt", "source"); p.contents.set("right/cache/a.txt", "old destination");
  for (const path of ["left", "right", "left/cache", "right/cache"]) p.folders.add(path);
  p.render(); act(() => open.open("/right/cache/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old destination"));
  act(() => open.close("/right/cache/a.txt"));
  await act(async () => { await commands.removeEntry("edge~data", "/right/cache"); open.removed("/right/cache"); });
  act(() => open.open("/left/cache/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("source"));
  const change = { kind: "move" as const, path: "/left/cache", parent: "/right" };
  await act(async () => {
    const result = await p.apply([change]);
    open.rebase("r2", result.fileTreeRevision, touchedPaths(change));
    open.renamed("/left/cache", "/right/cache");
    await client.invalidateQueries({ queryKey: ["files", "edge~data"], predicate: (q) => q.queryKey[2] !== "content" });
  });
  await waitFor(() => expect(open.files[0]?.path).toBe("/right/cache/a.txt"));
  await waitFor(() => expect(open.files[0]?.status === "ready" && client.isFetching() === 0).toBe(true));
  expect(open.files[0]?.saved).toBe("source");
  act(() => open.edit("/right/cache/a.txt", "source edit"));
  await act(async () => { await open.save("/right/cache/a.txt"); });
  expect(p.contents.get("right/cache/a.txt")).toBe("source edit"); expect(p.conflicts).toEqual([]);
});

it("keeps the baseline of edits made during a pending save fresh through another tree reload", async () => {
  const p = fixture(true); p.render(); act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  act(() => open.edit("/a.txt", "submitted"));
  p.hooks.updated = () => { p.hooks.updated = undefined; open.edit("/a.txt", "newer draft"); };
  await act(async () => { await open.save("/a.txt"); });
  await waitFor(() => expect(open.files[0]?.text).toBe("newer draft"));
  p.externalFolder("external");
  await act(async () => { await client.invalidateQueries({ queryKey: ["files", "edge~data"], predicate: (q) => q.queryKey[2] !== "content" }); });
  await act(async () => { await open.save("/a.txt"); });
  expect(p.contents.get("a.txt")).toBe("newer draft"); expect(p.conflicts).toEqual([]);
});

it("a dirty renamed tab re-reads its new path before advancing its original baseline", async () => {
  const p = fixture(true); p.render(); act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  act(() => open.edit("/a.txt", "my draft"));
  await act(async () => { await commands.rename("edge~data", "/a.txt", "renamed.txt"); open.renamed("/a.txt", "/renamed.txt"); });
  await waitFor(() => expect(open.files[0]?.path).toBe("/renamed.txt"));
  await waitFor(() => expect(client.getQueryData<VolumeFileContent>(["files", "edge~data", "content", "/renamed.txt"])?.revision).toBe("r2"));
  expect(open.files[0]?.text).toBe("my draft");
  await act(async () => { await open.save("/renamed.txt"); });
  expect(p.contents.get("renamed.txt")).toBe("my draft"); expect(p.conflicts).toEqual([]);
});
it.each([false, true])("reopening an unchanged closed tab after own gestures uses a current baseline (external elsewhere=%s)", async (external) => {
  const p = fixture(true); p.render(); act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  act(() => open.close("/a.txt"));
  if (external) {
    p.externalFolder("external");
    await act(async () => { await client.invalidateQueries({ queryKey: ["files", "edge~data"], predicate: (q) => q.queryKey[2] !== "content" }); });
  }
  await waitFor(() => expect(displayedRevision).toBe(p.revision()));
  await act(async () => { expect(await commands.createEntry("edge~data", "/", "own", "dir")).toBe(true); });
  act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.status === "ready" && client.isFetching() === 0).toBe(true));
  act(() => open.edit("/a.txt", "reopened edit"));
  await act(async () => { await open.save("/a.txt").catch(() => {}); });
  expect(p.conflicts).toEqual([]); expect(p.contents.get("a.txt")).toBe("reopened edit");
});

it("clears a dirty tab conflict after reload proves its entry unchanged, preserving the draft for retry", async () => {
  const p = fixture(true); p.render(); act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  act(() => open.edit("/a.txt", "my draft"));
  p.externalFolder("unrelated");
  await act(async () => { await expect(open.save("/a.txt")).rejects.toThrow(); });
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(open.files[0]?.conflict).toBe(false);
  expect(open.files[0]?.text).toBe("my draft");
  expect(open.files[0]?.saved).toBe("old A");
  await act(async () => { expect(await open.save("/a.txt")).toBe(true); });
  expect(p.contents.get("a.txt")).toBe("my draft");
  expect(p.conflicts).toEqual(["r1"]);
});

it("keeps a dirty tab conflicted when the reload finds its entry changed", async () => {
  const p = fixture(true); p.render(); act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  act(() => open.edit("/a.txt", "my draft"));
  p.externalWrite();
  await act(async () => { await expect(open.save("/a.txt")).rejects.toThrow(); });
  expect(open.files[0]?.conflict).toBe(true);
  expect(open.files[0]?.text).toBe("my draft");
  await act(async () => { expect(await open.save("/a.txt")).toBe(false); });
  expect(p.contents.get("a.txt")).toBe("external");
});

it("the producer baseline still rejects a retry after an identity-preserving external write", async () => {
  const p = fixture(true); p.render(); act(() => open.open("/a.txt"));
  await waitFor(() => expect(open.files[0]?.saved).toBe("old A"));
  act(() => open.edit("/a.txt", "my draft"));
  // Same length and mtime: the tree heuristic cannot see these changed bytes.
  p.contents.set("a.txt", "new A"); p.externalFolder("unrelated");
  await act(async () => { await expect(open.save("/a.txt")).rejects.toThrow(); });
  expect(open.files[0]?.conflict).toBe(false);
  await act(async () => { await expect(open.save("/a.txt")).rejects.toThrow(); });
  expect(p.contents.get("a.txt")).toBe("new A");
  expect(p.conflicts).toEqual(["r1", "r2"]);
});
