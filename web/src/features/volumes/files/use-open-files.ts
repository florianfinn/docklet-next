import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { errorCode } from "@/data/api";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { FileContent } from "@/domain/files";

export interface OpenFile {
  path: string;
  status: "loading" | "error" | "ready";
  errorCode?: string;
  /** Last saved text; empty for non-text files. */
  saved: string;
  /** Text shown in the editor, including unsaved edits. */
  text: string;
  editable: boolean;
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
  const [drafts, setDrafts] = useState<Record<string, string>>({});
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
    const data = q?.data as FileContent | undefined;
    const saved = data?.content ?? "";
    const editable = !!data?.editable;
    const draft = drafts[path];
    return {
      path,
      status: q?.isError ? "error" : data ? "ready" : "loading",
      errorCode: q?.isError ? (errorCode(q.error) ?? undefined) : undefined,
      saved,
      text: draft ?? saved,
      editable,
      size: data?.size ?? 0,
      dirty: editable && draft !== undefined && draft !== saved,
      retry: () => void q?.refetch(),
    };
  });

  const open = useCallback((path: string) => {
    setPaths((p) => (p.includes(path) ? p : [...p, path]));
    setClosed((c) => c.filter((x) => x !== path));
  }, []);
  const edit = useCallback((path: string, text: string) => setDrafts((d) => ({ ...d, [path]: text })), []);
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

  /** Writes the draft; the saved text then equals it. */
  const save = async (path: string) => {
    const file = files.find((f) => f.path === path);
    if (!file?.dirty) return false;
    await adapter.writeFile(volumeId, path, file.text);
    client.setQueryData<FileContent>(queryKeys.fileContent(volumeId, path), (prev) => (prev ? { ...prev, content: file.text, size: new TextEncoder().encode(file.text).length } : prev));
    discard(path);
    return true;
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

  return { files, open, edit, discard, close, save, renamed, removed, dirty: files.some((f) => f.dirty) };
}

export { moved as movedPath, under as isUnder };
