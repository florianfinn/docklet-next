import { AlertTriangle, CheckCircle2, Download, FileCode2, KeyRound, RotateCcw } from "lucide-react";
import { useMemo } from "react";
import { useTranslations } from "use-intl";

import { downloadBlob } from "@/shared/lib/tar";
import { CodeEditor } from "@/shared/workspace/code-editor";
import { EditorAction, EditorPane } from "@/shared/workspace/workspace";

import type { WorkspaceFile } from "./ComposeFileTree";
import { toDiagnostics, useEditorText } from "./editor-text";
import type { ComposeDraft } from "./use-compose-draft";

interface AdvancedEditorProps {
  draft: ComposeDraft;
  file: WorkspaceFile;
  onFile: (f: WorkspaceFile) => void;
  canReset: boolean;
  onSave?: () => void;
}

/** Compose adapter for the shared editor pane: compose.yaml and .env tabs, YAML diagnostics, sync status. */
export function AdvancedEditor({ draft, file, onFile, canReset, onSave }: AdvancedEditorProps) {
  const t = useTranslations("editor");
  const text = useEditorText();
  const parseErrors = draft.parsed.ok ? [] : draft.parsed.errors;
  const diagnostics = useMemo(
    () => (file === "compose" ? toDiagnostics(draft.compose, parseErrors, draft.issues, text.issue, text.parseError) : []),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- texts are stable per locale
    [file, draft.compose, draft.parsed, draft.issues],
  );
  const name = file === "compose" ? "compose.yaml" : ".env";
  const value = file === "compose" ? draft.compose : draft.env;
  const lines = value.split("\n").length;

  return (
    <EditorPane
      label={t("files.open")}
      tabs={[
        { id: "compose", label: "compose.yaml", icon: <FileCode2 />, dirty: draft.composeDirty },
        { id: "env", label: ".env", icon: <KeyRound />, dirty: draft.envDirty },
      ]}
      activeId={file}
      onSelect={(id) => onFile(id as WorkspaceFile)}
      actions={
        <>
          {canReset && <EditorAction label={t("discard")} icon={<RotateCcw />} onClick={draft.reset} disabled={!draft.dirty} />}
          <EditorAction label={t("download")} icon={<Download />} onClick={() => downloadBlob(name, value, "text/plain")} />
        </>
      }
      status={
        file === "env" ? (
          <span>{t("raw.envStatus", { count: lines })}</span>
        ) : draft.parsed.ok ? (
          <>
            <span className="inline-flex items-center gap-1 text-success">
              <CheckCircle2 className="size-3.5" aria-hidden /> {t("valid")}
            </span>
            <span>{t("raw.statusValid", { name: draft.parsed.model.name || "—", count: draft.parsed.model.services.length })}</span>
          </>
        ) : (
          <>
            <span className="inline-flex items-center gap-1 text-destructive">
              <AlertTriangle className="size-3.5" aria-hidden /> {t("raw.statusInvalid")}
            </span>
            {draft.parsed.errors.slice(0, 2).map((e, i) => (
              <span key={i} className="font-mono">
                {text.parseError(e)}
              </span>
            ))}
          </>
        )
      }
    >
      <CodeEditor
        key={file}
        value={value}
        onChange={file === "compose" ? draft.setCompose : draft.setEnv}
        language={file === "compose" ? "yaml" : "env"}
        diagnostics={diagnostics}
        label={t("raw.label", { file: name })}
        describedBy="raw-help"
        onSave={onSave}
      />
    </EditorPane>
  );
}
