import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";

import { errorCode, type VolumeFileContent, type WorkspaceBaseline, type WorkspaceReadOnlyReason } from "@/data/api";
import { WorkspaceConflictError } from "@/data/http/workspace";
import { workspaceErrorKey } from "./workspace-error";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";

export interface OpenFile {
  path: string;
  status: "loading" | "error" | "ready";
  errorCode?: string;
  /** Last saved text; empty for non-text files. */
  saved: string;
  /** Text shown in the editor, including unsaved edits. */
  text: string;
  editable: boolean;
  readOnlyReason?: WorkspaceReadOnlyReason;
  conflict?: boolean;
  size: number;
  dirty: boolean;
  retry: () => void;
}

const under = (path: string, root: string) => path === root || path.startsWith(`${root}/`);
const moved = (path: string, from: string, to: string) => (under(path, from) ? to + path.slice(from.length) : path);

/**
 * Open-file tabs of a volume workspace. Contents load through queries;
 * unsaved edits live here, so tab switches and folder navigation keep them.
 */
export function useOpenFiles(volumeId: string, fileParam: string | null) {
  const adapter = useAdapter();
  const client = useQueryClient();
  const [paths, setPaths] = useState<string[]>(() => (fileParam ? [fileParam] : []));
  const [drafts, setDrafts] = useState<Record<string, { text: string; baseline?: WorkspaceBaseline; conflict?: boolean }>>({});
  const pending = useRef(new Set<string>());
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  // Tabs closed since the last URL change: a navigation that was still pending when
  // its tab got closed (fast click on the close mark) must not reopen it.
  const [closed, setClosed] = useState<string[]>([]);
  // A file named in the URL (link, back/forward) joins the tabs once per change.
  const [seenParam, setSeenParam] = useState(fileParam);
  if (fileParam !== seenParam) {
    setSeenParam(fileParam);
    if (fileParam && !paths.includes(fileParam) && !closed.includes(fileParam)) setPaths([...paths, fileParam]);
    if (closed.length) setClosed([]);
  }

  const results = useQueries({
    queries: paths.map((path) => ({ queryKey: queryKeys.fileContent(volumeId, path), queryFn: () => adapter.readFile(volumeId, path), retry: false })),
  });

  const files: OpenFile[] = paths.map((path, i) => {
    const q = results[i];
    const data = q?.data as VolumeFileContent | undefined;
    const draft = drafts[path];
    const saved = draft?.baseline?.content ?? data?.content ?? "";
    const editable = !!data?.editable || !!draft;
    return {
      path,
      status: draft ? "ready" : q?.isError ? "error" : data ? "ready" : "loading",
      errorCode: q?.isError ? (errorCode(q.error) ?? workspaceErrorKey(q.error)) : undefined,
      saved,
      text: draft?.text ?? saved,
      readOnlyReason: data?.readOnlyReason,
      conflict: draft?.conflict,
      editable,
      size: data?.size ?? 0,
      dirty: draft !== undefined && (draft.text !== saved || !!draft.conflict),
      retry: () => void q?.refetch(),
    };
  });

  const open = useCallback((path: string) => {
    setPaths((p) => (p.includes(path) ? p : [...p, path]));
    setClosed((c) => c.filter((x) => x !== path));
  }, []);
  const edit = useCallback((path: string, text: string) => {
    const data = client.getQueryData<VolumeFileContent>(queryKeys.fileContent(volumeId, path));
    setDrafts((d) => {
      const next = { ...d, [path]: { ...d[path], text, baseline: d[path]?.baseline ?? (data?.revision ? { content: data.content, revision: data.revision } : undefined) } };
      return next;
    });
  }, [client, volumeId]);
  const discard = useCallback(
    (path: string) =>
      setDrafts((d) => {
        const { [path]: _drop, ...rest } = d;
        return rest;
      }),
    [],
  );
  const close = useCallback(
    (path: string) => {
      setPaths((p) => p.filter((x) => x !== path));
      setClosed((c) => [...c, path]);
      discard(path);
    },
    [discard],
  );

  // A successful manifest proves that only our change separates these revisions.
  const rebase = useCallback((from: string, to: string) => {
    client.setQueriesData<VolumeFileContent>({ queryKey: ["files", volumeId, "content"] }, (data) => data?.revision === from ? { ...data, revision: to } : data);
    setDrafts((current) => Object.fromEntries(Object.entries(current).map(([path, draft]) => [path, draft.baseline?.revision === from ? { ...draft, baseline: { ...draft.baseline, revision: to } } : draft])));
  }, [client, volumeId]);

  /** Writes the draft; the saved text then equals it. */
  const save = async (path: string) => {
    const file = files.find((f) => f.path === path);
    if (!file?.dirty || file.conflict || pending.current.has(path)) return false;
    pending.current.add(path);
    try {
      const result = await adapter.writeFile(volumeId, path, file.text, drafts[path]?.baseline);
      if (!mounted.current) return false;
      if (result?.revision && drafts[path]?.baseline) rebase(drafts[path].baseline.revision, result.revision);
      client.setQueryData<VolumeFileContent>(queryKeys.fileContent(volumeId, path), (prev) => (prev ? { ...prev, content: file.text, revision: result?.revision ?? prev.revision, size: new TextEncoder().encode(file.text).length } : prev));
      setDrafts((d) => {
        const next = { ...d };
        if (next[path]?.text === file.text) delete next[path];
        else if (next[path]) next[path] = { ...next[path], baseline: result?.revision ? { content: file.text, revision: result.revision } : undefined };
        return next;
      });
      await client.invalidateQueries({ queryKey: ["files", volumeId], predicate: (q) => q.queryKey[2] !== "content" });
      return true;
    } catch (error) {
      if (error instanceof WorkspaceConflictError && mounted.current) {
        setDrafts((d) => ({ ...d, [path]: { ...d[path]!, conflict: true } }));
        await client.invalidateQueries({ queryKey: ["files", volumeId], predicate: (q) => q.queryKey[2] !== "content" });
        await client.refetchQueries({ queryKey: queryKeys.fileContent(volumeId, path), exact: true });
      }
      throw error;
    } finally { pending.current.delete(path); }
  };

  /** Keeps tabs and drafts attached after a rename of the file or one of its folders. */
  const renamed = useCallback((from: string, to: string) => {
    setPaths((p) => p.map((x) => moved(x, from, to)));
    setDrafts((d) => Object.fromEntries(Object.entries(d).map(([k, v]) => [moved(k, from, to), v])));
  }, []);

  const removed = useCallback((path: string) => {
    setPaths((p) => p.filter((x) => !under(x, path)));
    setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([k]) => !under(k, path))));
  }, []);

  return { files, open, edit, discard, close, save, rebase, renamed, removed, dirty: files.some((f) => f.dirty) };
}

export { moved as movedPath, under as isUnder };
