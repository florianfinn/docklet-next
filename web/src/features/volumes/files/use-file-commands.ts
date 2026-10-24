// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original volume workspace mutation lifecycle for issue #182.

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslations } from "use-intl";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { useAdapter } from "@/data/use-adapter";
import type { WorkspaceUpload, WorkspaceWriteResult, VolumeDirListing } from "@/data/api";
import { joinPath, parentPath } from "@/domain/files";
import { workspaceErrorKey } from "./workspace-error";

export function useFileCommands(id: string, revision: string | undefined, rebase: (from: string | undefined, to: string, touched: string[]) => void, writeLockRef: RefObject<boolean>) {
  const adapter = useAdapter();
  const client = useQueryClient();
  const t = useTranslations("volumes.files.workspaceErrors");
  const mounted = useRef(true);
  const [busy, setBusy] = useState(false);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  async function refresh() { await client.invalidateQueries({ queryKey: ["files", id], predicate: (query) => query.queryKey[2] !== "content" }); }
  async function run(fn: () => Promise<WorkspaceWriteResult>, touched: string[], expected = revision) {
    const failure = captureCommandFailure(adapter);
    if (writeLockRef.current) return false;
    writeLockRef.current = true; setBusy(true);
    try {
      const result = await fn();
      if (mounted.current && result?.revision) rebase(expected, result.revision, touched);
      await refresh();
      return mounted.current;
    } catch (error) {
      if (mounted.current || failure.closed()) { failure.error(t(workspaceErrorKey(error))); await refresh(); }
      return false;
    } finally { writeLockRef.current = false; if (mounted.current) setBusy(false); }
  }
  return {
    busy,
    createEntry: (_id: string, dir: string, name: string, type: "dir" | "file") => run(() => adapter.createEntry(id, dir, name, type, revision), [joinPath(dir, name)]),
    rename: (_id: string, path: string, name: string) => run(() => adapter.renameEntry(id, path, name, revision), [path, joinPath(parentPath(path), name)]),
    removeEntry: (_id: string, path: string) => run(() => adapter.deleteEntry(id, path, revision), [path]),
    upload: (_id: string, dir: string, file: WorkspaceUpload) => {
      const expected = client.getQueryData<VolumeDirListing>(["files", id, dir])?.revision ?? revision;
      return run(() => adapter.uploadFile(id, dir, file, expected), [joinPath(dir, file.name)], expected);
    },
    async download(path: string) {
      const failure = captureCommandFailure(adapter);
      try {
        if (adapter.downloadFile) { const blob = await adapter.downloadFile(id, path); return mounted.current ? blob : undefined; }
        const file = await adapter.readFile(id, path);
        return mounted.current && file.editable ? new Blob([file.content], { type: "text/plain" }) : undefined;
      } catch (error) { if (mounted.current || failure.closed()) failure.error(t(workspaceErrorKey(error))); }
    },
  };
}
