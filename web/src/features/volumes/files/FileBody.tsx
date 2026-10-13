import { AlertTriangle, Download, FileLock2 } from "lucide-react";
import { useTranslations } from "use-intl";

import { baseName } from "@/domain/files";
import { formatBytes } from "@/domain/format";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";
import { CodeEditor } from "@/shared/workspace/code-editor";
import { languageFor } from "@/shared/workspace/languages";
import { EditorPlaceholder } from "@/shared/workspace/workspace";

import type { OpenFile } from "./use-open-files";

type ErrorKey = "not-found" | "host-offline" | "not-editable";
const KNOWN_ERRORS = new Set<string>(["not-found", "host-offline", "not-editable"]);

/** Editor body of one open file: loading, error, read-only placeholder or the shared CodeMirror editor. */
export function FileBody({ file, onEdit, onSave, onDownload }: { file: OpenFile; onEdit: (text: string) => void; onSave: () => void; onDownload: () => void }) {
  const t = useTranslations("volumes.files");
  const te = useTranslations("volumes.errors");
  const name = baseName(file.path);
  if (file.status === "loading") return <EditorPlaceholder icon={<Spinner />} title={t("loadingFile", { name })} />;
  if (file.status === "error")
    return (
      <EditorPlaceholder
        icon={<AlertTriangle className="text-destructive" />}
        title={t("loadFailed", { name })}
        description={file.errorCode && KNOWN_ERRORS.has(file.errorCode) ? te(file.errorCode as ErrorKey) : undefined}
        action={
          <Button size="sm" variant="outline" onClick={file.retry}>
            {t("retry")}
          </Button>
        }
      />
    );
  if (!file.editable)
    return (
      <EditorPlaceholder
        icon={<FileLock2 />}
        title={t("notEditable")}
        description={t("binaryBody", { size: formatBytes(file.size) })}
        action={
          <Button size="sm" variant="outline" onClick={onDownload}>
            <Download data-icon="inline-start" />
            {t("download")}
          </Button>
        }
      />
    );
  return <CodeEditor key={file.path} value={file.text} onChange={onEdit} language={languageFor(name)} label={t("editorLabel", { name })} describedBy="file-editor-help" onSave={onSave} />;
}

/** Status line of the active file tab. */
export function FileStatus({ file }: { file: OpenFile }) {
  const t = useTranslations("volumes.files");
  const tw = useTranslations("workspace");
  if (file.status !== "ready") return <span className="font-mono">{file.path}</span>;
  if (!file.editable)
    return (
      <>
        <span>{tw("readOnly")}</span>
        <span className="font-mono">{formatBytes(file.size)}</span>
        <span className="truncate font-mono">{file.path}</span>
      </>
    );
  return (
    <>
      <span className="font-medium text-foreground/80">{tw(`languages.${languageFor(baseName(file.path))}`)}</span>
      <span>{tw("lines", { count: file.text.split("\n").length })}</span>
      <span className="font-mono">{formatBytes(new TextEncoder().encode(file.text).length)}</span>
      <span className={file.dirty ? "text-warning" : undefined} aria-live="polite">
        {file.dirty ? t("editorDirty") : t("editorClean")}
      </span>
      <span className="ml-auto hidden sm:inline">{tw("saveHint")}</span>
    </>
  );
}
