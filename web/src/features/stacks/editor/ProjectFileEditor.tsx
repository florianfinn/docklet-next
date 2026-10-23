// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original extra project file editor for issue #182.

import { useTranslations } from "use-intl";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { CodeEditor } from "@/shared/workspace/code-editor";
import { languageFor } from "@/shared/workspace/languages";
import { EditorPane, EditorPlaceholder } from "@/shared/workspace/workspace";
import { workspaceErrorKey } from "@/features/volumes/files/workspace-error";
import type { ProjectWorkspace } from "./use-project-workspace";

export function ProjectFileEditor({ workspace, readOnly, canDownload, onSave, onDownload }: { workspace: ProjectWorkspace; readOnly: boolean; canDownload: boolean; onSave: () => void; onDownload: () => void }) {
  const t = useTranslations("editor.extraFiles");
  const tv = useTranslations("volumes.files");
  const file = workspace.files.find((entry) => entry.path === workspace.active);
  return <EditorPane label={t("open")} tabs={workspace.files.map((entry) => ({ id: entry.path, label: entry.path, dirty: !!entry.draft && entry.draft.saved !== entry.draft.text }))} activeId={workspace.active ?? ""} onSelect={workspace.setActive}
    actions={<><Button size="sm" variant="ghost" disabled={!file?.draft || workspace.saving} onClick={() => file && workspace.discard(file.path)}>{t("discard")}</Button><Button size="sm" variant="ghost" disabled={!canDownload} onClick={onDownload}>{t("download")}</Button></>}>
    {!file || file.pending ? <p role="status">{t("loading")}</p> : file.error && !file.draft ? <EditorPlaceholder icon={null} title={tv(`workspaceErrors.${workspaceErrorKey(file.error)}`)} action={<Button onClick={() => void file.retry()}>{t("retry")}</Button>} /> : file.draft || file.data?.editable ? <>
      {file.draft?.conflict && <Alert><AlertDescription>{tv("workspaceErrors.conflict")}</AlertDescription></Alert>}
      <CodeEditor key={`${file.path}:${readOnly}`} value={file.draft?.text ?? file.data?.content ?? ""} language={languageFor(file.path)} label={t("content", { path: file.path })} onChange={(text) => workspace.edit(file.path, text)} onSave={readOnly || file.draft?.conflict ? undefined : onSave} readOnly={readOnly} />
    </> : <EditorPlaceholder icon={null} title={t("readOnly")} description={tv(`readOnlyReasons.${file.data?.readOnlyReason ?? "binary"}`)} action={<Button disabled={!canDownload} onClick={onDownload}>{t("download")}</Button>} />}
  </EditorPane>;
}
