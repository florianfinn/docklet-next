// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original volume workspace mutation lifecycle for issue #182.

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import { toast } from "sonner";
import { useAdapter } from "@/data/use-adapter";
import type { WorkspaceUpload, VolumeDirListing } from "@/data/api";
import { workspaceErrorKey } from "./workspace-error";

export function useFileCommands(id: string, revision?: string) {
  const adapter = useAdapter();
  const client = useQueryClient();
  const t = useTranslations("volumes.files.workspaceErrors");
  const mounted = useRef(true);
  const lock = useRef(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  async function refresh() { await client.invalidateQueries({ queryKey: ["files", id], predicate: (query) => query.queryKey[2] !== "content" }); }
  async function run(fn: () => Promise<unknown>) {
    if (lock.current) return false;
    lock.current = true; setBusy(true);
    try {
      await fn(); await refresh();
      return mounted.current;
    } catch (error) {
      if (mounted.current) { toast.error(t(workspaceErrorKey(error))); await refresh(); }
      return false;
    } finally { lock.current = false; if (mounted.current) setBusy(false); }
  }
  return {
    busy,
    createEntry: (_id: string, dir: string, name: string, type: "dir" | "file") => run(() => adapter.createEntry(id, dir, name, type, revision)),
    rename: (_id: string, path: string, name: string) => run(() => adapter.renameEntry(id, path, name, revision)),
    removeEntry: (_id: string, path: string) => run(() => adapter.deleteEntry(id, path, revision)),
    upload: (_id: string, dir: string, file: WorkspaceUpload) => run(() => adapter.uploadFile(id, dir, file, client.getQueryData<VolumeDirListing>(["files", id, dir])?.revision ?? revision)),
    async download(path: string) {
      try {
        if (adapter.downloadFile) { const blob = await adapter.downloadFile(id, path); return mounted.current ? blob : undefined; }
        const file = await adapter.readFile(id, path);
        return mounted.current && file.editable ? new Blob([file.content], { type: "text/plain" }) : undefined;
      } catch (error) { if (mounted.current) toast.error(t(workspaceErrorKey(error))); }
    },
  };
}
