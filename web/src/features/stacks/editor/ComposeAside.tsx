import { AlertCircle, AlertTriangle, Box, CheckCircle2, Database, FileCode2, FolderOpen, KeyRound } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useVolumes } from "@/data/queries";
import { mountRef } from "@/domain/volumes";
import { filesHref } from "@/features/volumes/files/files-href";
import { cn } from "@/shared/lib/utils";
import { CodeEditor } from "@/shared/workspace/code-editor";
import { AsideHeader, EditorPane, type AsideMode } from "@/shared/workspace/workspace";

import type { WorkspaceFile } from "./ComposeFileTree";
import { useEditorText } from "./editor-text";
import type { ComposeDraft } from "./use-compose-draft";

interface ComposeAsideProps {
  mode: Exclude<AsideMode, "off">;
  draft: ComposeDraft;
  file: WorkspaceFile;
  hostId: string;
  stackId: string | null;
  hostProjectDir?: string;
  onFile: (f: WorkspaceFile) => void;
}

/** Validation, service outline and mounts of the draft, next to the editor. */
function Details({ draft, hostId, stackId, hostProjectDir, onFile }: Omit<ComposeAsideProps, "mode" | "file">) {
  const t = useTranslations("editor.aside");
  const text = useEditorText();
  const volumes = useVolumes();
  const known = new Map((volumes.data ?? []).map((v) => [v.id, v]));
  const model = draft.parsed.ok ? draft.parsed.model : null;
  const errors = draft.parsed.ok ? draft.issues : [];
  return (
    <div className="grid min-h-0 flex-1 auto-rows-min gap-4 overflow-auto p-3 @min-[3000px]/ws:grid-cols-2">
      <section aria-labelledby="ca-check" className="min-w-0">
        <h3 id="ca-check" className="mb-1.5 text-xs font-semibold text-muted-foreground">
          {t("check")}
        </h3>
        {!draft.parsed.ok ? (
          <ul className="flex flex-col gap-1">
            {draft.parsed.errors.map((e, i) => (
              <li key={i} className="flex items-start gap-1.5 text-xs text-destructive">
                <AlertCircle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
                <span className="font-mono">{text.parseError(e)}</span>
              </li>
            ))}
          </ul>
        ) : errors.length === 0 ? (
          <p className="flex items-center gap-1.5 text-xs text-success">
            <CheckCircle2 className="size-3.5" aria-hidden />
            {t("clean")}
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {errors.map((i, n) => (
              <li key={n}>
                <button type="button" onClick={() => onFile("compose")} className={cn("flex w-full items-start gap-1.5 rounded-md px-1 py-0.5 text-left text-xs hover:bg-muted/50", i.level === "error" ? "text-destructive" : "text-warning")}>
                  {i.level === "error" ? <AlertCircle className="mt-0.5 size-3.5 shrink-0" aria-hidden /> : <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />}
                  <span>
                    {i.service && <span className="font-mono">{i.service}: </span>}
                    {text.issue(i)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      {model && (
        <section aria-labelledby="ca-services" className="min-w-0">
          <h3 id="ca-services" className="mb-1.5 text-xs font-semibold text-muted-foreground">
            {t("services", { count: model.services.length })}
          </h3>
          <ul className="flex flex-col gap-1.5">
            {model.services.map((s) => (
              <li key={s.key} className="rounded-lg border bg-muted/15 px-2.5 py-1.5 text-xs">
                <div className="flex items-center gap-1.5 font-medium">
                  <Box className="size-3.5 text-muted-foreground" aria-hidden />
                  {s.key}
                </div>
                <div className="truncate font-mono text-[11px] text-muted-foreground">{s.image || "—"}</div>
                <div className="mt-0.5 text-[11px] text-muted-foreground">{t("serviceMeta", { ports: s.ports.length, mounts: s.mounts.length, env: s.env.length })}</div>
              </li>
            ))}
          </ul>
        </section>
      )}
      {model && (
        <section aria-labelledby="ca-mounts" className="min-w-0">
          <h3 id="ca-mounts" className="mb-1.5 text-xs font-semibold text-muted-foreground">
            {t("mounts")}
          </h3>
          {(() => {
            const mounts = model.services.flatMap((s) => s.mounts.filter((m) => m.editable && m.source && m.target).map((m) => ({ service: s.key, ...m })));
            if (!mounts.length) return <p className="text-xs text-muted-foreground">{t("noMounts")}</p>;
            return (
              <ul className="flex flex-col gap-1">
                {mounts.map((m) => {
                  const ref = stackId ? mountRef({ hostId, stackId }, `${m.source}:${m.target}`, hostProjectDir) : undefined;
                  const volume = ref ? known.get(ref.id) : undefined;
                  const bind = /^[./~]/.test(m.source);
                  return (
                    <li key={`${m.service}-${m.index}`} className="flex items-center gap-1.5 text-xs">
                      {bind ? <FolderOpen className="size-3.5 shrink-0 text-muted-foreground" aria-hidden /> : <Database className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />}
                      {volume ? (
                        <Link to={filesHref(volume)} className="truncate font-mono text-primary-text hover:underline">
                          {m.source}
                        </Link>
                      ) : (
                        <span className="truncate font-mono">{m.source}</span>
                      )}
                      <span className="text-muted-foreground">→</span>
                      <span className="truncate font-mono text-muted-foreground">{m.target}</span>
                    </li>
                  );
                })}
              </ul>
            );
          })()}
        </section>
      )}
    </div>
  );
}

/** Second file of the stack beside the editor; edits go into the same draft. */
function SecondFile({ draft, file, onFile }: Pick<ComposeAsideProps, "draft" | "file" | "onFile">) {
  const t = useTranslations("editor");
  const other: WorkspaceFile = file === "compose" ? "env" : "compose";
  const name = other === "compose" ? "compose.yaml" : ".env";
  return (
    <EditorPane
      label={t("aside.secondary")}
      tabs={[{ id: other, label: name, icon: other === "compose" ? <FileCode2 /> : <KeyRound />, dirty: other === "compose" ? draft.composeDirty : draft.envDirty }]}
      activeId={other}
      onSelect={() => onFile(other)}
    >
      <CodeEditor key={`aside-${other}`} value={other === "compose" ? draft.compose : draft.env} onChange={other === "compose" ? draft.setCompose : draft.setEnv} language={other === "compose" ? "yaml" : "env"} label={t("raw.label", { file: name })} />
    </EditorPane>
  );
}

export function ComposeAside(props: ComposeAsideProps) {
  const t = useTranslations("editor.aside");
  if (props.mode === "split") return <SecondFile draft={props.draft} file={props.file} onFile={props.onFile} />;
  return (
    <>
      <AsideHeader title={t("title")} />
      <Details draft={props.draft} hostId={props.hostId} stackId={props.stackId} hostProjectDir={props.hostProjectDir} onFile={props.onFile} />
    </>
  );
}
