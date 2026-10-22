// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original project tree gestures for issue #182.

import { useRef, useState } from "react";
import { useTranslations } from "use-intl";
import { baseName, parentPath } from "@/domain/files";
import { useCapability } from "@/data/use-capability";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { TextField } from "@/shared/components/form-field";
import { FieldGroup } from "@/shared/ui/field";
import type { ProjectWorkspace } from "./use-project-workspace";

export function ProjectFileActions({ workspace, id, busy, report }: { workspace: ProjectWorkspace; id: string; busy: boolean; report: (error: unknown) => void }) {
  const t = useTranslations("editor.extraFiles");
  const capability = useCapability("updateProjectWorkspace", { kind: "stack", id });
  const upload = useRef<HTMLInputElement>(null);
  const [dialog, setDialog] = useState<"file" | "folder" | "rename" | "delete" | null>(null);
  const [value, setValue] = useState("");
  const [error, setError] = useState(false);
  const selected = workspace.tree.data?.files?.find((entry) => entry.relativePath === workspace.selected);
  const directory = selected?.isDirectory ? selected.relativePath : selected ? parentPath(selected.relativePath).replace(/^\//, "") : "";
  const disabled = busy || workspace.saving || workspace.dirty || !workspace.tree.data || workspace.tree.isError || !capability.available;
  const open = (kind: typeof dialog) => { setError(false); setValue(kind === "rename" ? baseName(workspace.selected ?? "") : directory ? `${directory}/` : ""); setDialog(kind); };
  async function apply() {
    try {
      if (!value.trim()) { setError(true); return; }
      await workspace.gesture(dialog === "rename" ? { kind: "rename", path: workspace.selected!, name: value } : dialog === "folder" ? { kind: "create-folder", path: value } : { kind: "create-file", path: value, content: "" });
      setDialog(null);
    } catch (cause) { setError(true); report(cause); }
  }
  return <div className="flex flex-wrap items-center gap-2" aria-label={t("title")}>
    <Button size="sm" variant="outline" disabled={disabled} title={workspace.dirty ? t("saveFirst") : capability.reason} onClick={() => open("file")}>{t("create")}</Button>
    <Button size="sm" variant="outline" disabled={disabled} onClick={() => open("folder")}>{t("folder")}</Button>
    <Button size="sm" variant="outline" disabled={disabled || !selected || selected.isSymlink} onClick={() => open("rename")}>{t("rename")}</Button>
    <Button size="sm" variant="outline" disabled={disabled || !selected || selected.isSymlink} onClick={() => open("delete")}>{t("delete")}</Button>
    <Button size="sm" variant="outline" disabled={disabled} onClick={() => upload.current?.click()}>{t("upload")}</Button>
    <input type="file" multiple ref={upload} className="sr-only" tabIndex={-1} aria-hidden onChange={(event) => {
      const files = [...event.target.files ?? []]; event.target.value = "";
      if (disabled || !files.length) return;
      void workspace.gesture(files.map((file) => ({ kind: "upload" as const, path: `${directory ? `${directory}/` : ""}${file.name}`, file: { name: file.name, size: file.size, blob: file } }))).catch(report);
    }} />
    {workspace.tree.isError && <Button size="sm" onClick={() => void workspace.tree.refetch()}>{t("retry")}</Button>}
    <Dialog open={dialog !== null && dialog !== "delete"} onOpenChange={(on) => !on && setDialog(null)}>
      <DialogContent><DialogHeader><DialogTitle>{dialog === "rename" ? t("rename") : dialog === "folder" ? t("folder") : t("create")}</DialogTitle><DialogDescription>{t("pathHint")}</DialogDescription></DialogHeader>
        <form onSubmit={(event) => { event.preventDefault(); void apply(); }} className="flex flex-col gap-3">
          <FieldGroup><TextField id="project-file-path" label={dialog === "rename" ? t("name") : t("path")} value={value} onChange={setValue} error={error ? t("pathInvalid") : undefined} mono /></FieldGroup>
          <Button type="submit" disabled={workspace.saving || !value.trim()}>{t("apply")}</Button>
        </form>
      </DialogContent>
    </Dialog>
    <ConfirmDialog open={dialog === "delete"} onOpenChange={(on) => !on && setDialog(null)} title={t("removeTitle", { path: workspace.selected ?? "" })} description={t("removeBody")} confirmLabel={t("delete")} destructive onConfirm={async () => { try { await workspace.gesture({ kind: "delete", path: workspace.selected! }); } catch (cause) { report(cause); } }} />
  </div>;
}
