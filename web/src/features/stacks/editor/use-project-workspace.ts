// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original project file draft and request lifecycle for issue #182.

import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { WorkspaceContent, WorkspaceGesture, WorkspaceTree } from "@/data/api";
import { WorkspaceConflictError, WorkspaceError } from "@/data/http/workspace";
import { useAdapter } from "@/data/use-adapter";
import type { TreeNode } from "@/shared/workspace/file-tree";

interface Draft { text: string; saved: string; revision: string; conflict?: boolean }
const fileKey = (id: string, path: string) => ["project-workspace-file", id, path];
export function useProjectWorkspace(id: string | null, rawFile: "compose" | "env") {
  const adapter = useAdapter();
  const client = useQueryClient();
  const key = ["project-workspace", id];
  const supported = !!id && !!adapter.getProjectWorkspace;
  const tree = useQuery({ queryKey: key, queryFn: () => adapter.getProjectWorkspace!(id!), enabled: supported, retry: false });
  const [paths, setPaths] = useState<string[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [active, setActiveState] = useState<string | null>(null);
  function setActive(path: string | null) { setActiveState(path); if (path !== null) setSelected(path); }
  const activeView = useRef(`${active ?? rawFile}:${selected ?? ""}`);
  useLayoutEffect(() => { activeView.current = `${active ?? rawFile}:${selected ?? ""}`; }, [active, rawFile, selected]);
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const latestDrafts = useRef(drafts);
  useLayoutEffect(() => { latestDrafts.current = drafts; }, [drafts]);
  const [saving, setSaving] = useState(false);
  const lock = useRef(false);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const results = useQueries({ queries: paths.map((path) => ({ queryKey: fileKey(id!, path), queryFn: () => adapter.readProjectFile!(id!, path), retry: false })) });
  const files = paths.map((path, index) => ({ path, data: results[index]?.data, error: results[index]?.error, pending: results[index]?.isPending, retry: () => results[index]?.refetch(), draft: drafts[path] }));
  const dirty = Object.entries(drafts).some(([, draft]) => draft.text !== draft.saved);
  function open(path: string) { setSelected(path); setPaths((current) => current.includes(path) ? current : [...current, path]); setActive(path); }
  function edit(path: string, text: string) {
    const data = client.getQueryData<WorkspaceContent>(fileKey(id!, path));
    setDrafts((current) => ({ ...current, [path]: current[path] ? { ...current[path], text } : { text, saved: data?.content ?? "", revision: tree.data!.fileTreeRevision } }));
  }
  function discard(path: string) { setDrafts((current) => { const next = { ...current }; delete next[path]; return next; }); }
  async function update(changes: WorkspaceGesture[], revision: string) {
    try {
      const next = await adapter.updateProjectWorkspace!(id!, revision, changes);
      if (mounted.current) client.setQueryData<WorkspaceTree>(key, next);
      return next;
    } catch (error) {
      if (error instanceof WorkspaceConflictError && mounted.current) {
          setDrafts((current) => Object.fromEntries(Object.entries(current).map(([path, draft]) => [path, { ...draft, conflict: true }])));
        await Promise.all([tree.refetch(), ...paths.map((path) => client.refetchQueries({ queryKey: fileKey(id!, path), exact: true }))]);
      }
      throw error;
    }
  }
  async function save() {
    const changed = Object.entries(drafts).filter(([, draft]) => draft.text !== draft.saved);
    if (!changed.length) return true;
    if (lock.current || changed.some(([, draft]) => draft.conflict)) throw new WorkspaceError("invalid");
    // Every update carries both its opened tree revision and original text. A
    // structural change after drafting requires an explicit conflict resolution.
    const revision = changed[0]![1].revision;
    lock.current = true; setSaving(true);
    try {
      const next = await update(changed.map(([path, draft]) => ({ kind: "update", path, content: draft.text, baseline: draft.saved })), revision);
      if (!mounted.current) return false;
      const complete = changed.every(([path, draft]) => latestDrafts.current[path]?.text === draft.text);
      for (const [path, draft] of changed) client.setQueryData<WorkspaceContent>(fileKey(id!, path), (value) => value ? { ...value, content: draft.text, size: new TextEncoder().encode(draft.text).length } : value);
      setDrafts((current) => {
        const remaining = { ...current };
        for (const [path, submitted] of changed) {
          if (remaining[path]?.text === submitted.text) delete remaining[path];
          else if (remaining[path]) remaining[path] = { text: remaining[path].text, saved: submitted.text, revision: next.fileTreeRevision };
        }
        return remaining;
      });
      return complete;
    } finally { lock.current = false; if (mounted.current) setSaving(false); }
  }
  async function gesture(input: WorkspaceGesture | WorkspaceGesture[]) {
    const owner = activeView.current;
    const changes = Array.isArray(input) ? input : [input];
    if (!tree.data || lock.current) throw new WorkspaceError("invalid");
    lock.current = true; setSaving(true);
    try {
      await update(changes, tree.data.fileTreeRevision);
      if (!mounted.current || activeView.current !== owner) return;
      for (const change of changes) {
        if (change.kind === "create-file" || change.kind === "upload") open(change.path.replace(/^\/+/, ""));
        if (change.kind === "rename" || change.kind === "delete") {
          const under = (path: string) => path === change.path || path.startsWith(`${change.path}/`);
          setPaths((current) => current.filter((path) => !under(path)));
          setDrafts((current) => Object.fromEntries(Object.entries(current).filter(([path]) => !under(path))));
          if (active && under(active)) setActive(null);
          if (selected && under(selected)) setSelected(null);
        }
      }
    } finally { lock.current = false; if (mounted.current) setSaving(false); }
  }
  function nodes(parent = ""): TreeNode[] {
    return (tree.data?.files ?? []).filter((entry) => entry.relativePath.split("/").slice(0, -1).join("/") === parent).map((entry) => ({
      id: `extra:${entry.relativePath}`, name: entry.name, kind: entry.isDirectory && !entry.isSymlink ? "dir" : "file",
      dirty: !!drafts[entry.relativePath] && drafts[entry.relativePath]!.text !== drafts[entry.relativePath]!.saved,
      children: entry.isDirectory && !entry.isSymlink ? nodes(entry.relativePath) : undefined,
    }));
  }
  return { supported, tree, active, setActive, selected, setSelected, files, dirty, open, edit, discard, save, gesture, saving, nodes: nodes(), conflicted: Object.values(drafts).some((draft) => draft.conflict) };
}
export type ProjectWorkspace = ReturnType<typeof useProjectWorkspace>;
