import { Box, FileText, Layers } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { baseName } from "@/domain/files";
import { formatBytes } from "@/domain/format";
import type { Volume } from "@/domain/volumes";
import { cn } from "@/shared/lib/utils";
import { languageFor } from "@/shared/workspace/languages";
import { AsideHeader, EditorPane, EditorPlaceholder, type AsideMode } from "@/shared/workspace/workspace";

import { FileBody } from "./FileBody";
import type { OpenFile } from "./use-open-files";

interface FileAsideProps {
  mode: Exclude<AsideMode, "off">;
  readOnly?: boolean;
  volume: Volume;
  /** Same warning the page shows above the workspace (running consumers, databases). */
  warning?: string;
  path: string;
  active: OpenFile | undefined;
  files: OpenFile[];
  /** Path shown in the second pane; never the active file. */
  secondary: string | null;
  onSecondary: (path: string) => void;
  onEdit: (path: string, text: string) => void;
  onSave: (file: OpenFile) => void;
  onDownload: (file: OpenFile) => void;
}

function Kv({ label, children, mono }: { label: string; children: React.ReactNode; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn("min-w-0 break-words", mono && "font-mono text-[11.5px]")}>{children}</dd>
    </>
  );
}

/** Context of the open file and of the volume: what it is, who mounts it, what to watch out for. */
function Details({ volume, path, active, warning }: Pick<FileAsideProps, "volume" | "path" | "active" | "warning">) {
  const t = useTranslations("volumes.files.aside");
  const tw = useTranslations("workspace");
  return (
    <div className="grid min-h-0 flex-1 auto-rows-min gap-4 overflow-auto p-3 text-xs @min-[3000px]/ws:grid-cols-2">
      <section aria-labelledby="fa-item" className="min-w-0">
        <h3 id="fa-item" className="mb-1.5 font-semibold text-muted-foreground">
          {active ? t("file") : t("folder")}
        </h3>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
          <Kv label={t("path")} mono>
            {active?.path ?? path}
          </Kv>
          {active?.status === "ready" && (
            <>
              <Kv label={t("kind")}>{active.editable ? tw(`languages.${languageFor(baseName(active.path))}`) : tw("readOnly")}</Kv>
              <Kv label={t("size")}>{formatBytes(active.editable ? new TextEncoder().encode(active.text).length : active.size)}</Kv>
              {active.editable && <Kv label={t("lines")}>{active.text.split("\n").length}</Kv>}
              <Kv label={t("state")}>{active.dirty ? <span className="text-warning">{t("dirty")}</span> : t("clean")}</Kv>
            </>
          )}
        </dl>
      </section>
      <section aria-labelledby="fa-consumers" className="min-w-0">
        <h3 id="fa-consumers" className="mb-1.5 font-semibold text-muted-foreground">
          {t("consumers", { count: volume.consumers.length })}
        </h3>
        {volume.consumers.length === 0 ? (
          <p className="text-muted-foreground">{t("unused")}</p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {volume.consumers.map((c) => (
              <li key={`${c.containerId}-${c.target}`} className="rounded-lg border bg-muted/15 px-2.5 py-1.5">
                <div className="flex items-center gap-1.5 font-medium">
                  {c.stackId ? <Layers className="size-3.5 text-muted-foreground" aria-hidden /> : <Box className="size-3.5 text-muted-foreground" aria-hidden />}
                  {c.stackId ? (
                    <Link to={`/stacks/${c.stackId}`} className="truncate hover:text-primary-text hover:underline">
                      {c.containerName}
                    </Link>
                  ) : (
                    <Link to={`/containers?c=${c.containerId}`} className="truncate hover:text-primary-text hover:underline">
                      {c.containerName}
                    </Link>
                  )}
                  <span className={cn("ml-auto text-[11px]", c.running ? "text-success" : "text-muted-foreground")}>{c.running ? t("running") : t("stopped")}</span>
                </div>
                <div className="font-mono text-[11px] text-muted-foreground">
                  → {c.target}
                  {c.readOnly && " · ro"}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section aria-labelledby="fa-notes" className="min-w-0">
        <h3 id="fa-notes" className="mb-1.5 font-semibold text-muted-foreground">
          {t("notes")}
        </h3>
        <ul className="flex list-disc flex-col gap-1 pl-4 text-muted-foreground">
          {warning && <li className="text-warning">{warning}</li>}
          <li>{t("source", { source: volume.source })}</li>
          <li>{t("memory")}</li>
        </ul>
      </section>
    </div>
  );
}

/** A second open file beside the editor; edits share the drafts of the tabs. */
function SecondFile({ readOnly, files, active, secondary, onSecondary, onEdit, onSave, onDownload }: Omit<FileAsideProps, "mode" | "volume" | "path" | "warning">) {
  const t = useTranslations("workspace.aside");
  const candidates = files.filter((f) => f.path !== active?.path);
  const file = candidates.find((f) => f.path === secondary) ?? candidates[0];
  if (!file) return <EditorPlaceholder icon={<FileText />} title={t("secondary")} description={t("none")} />;
  return (
    <EditorPane
      label={t("choose")}
      tabs={candidates.map((f) => ({ id: f.path, label: baseName(f.path), icon: <FileText />, dirty: f.dirty, title: f.path }))}
      activeId={file.path}
      onSelect={onSecondary}
    >
      <FileBody file={file} readOnly={readOnly} onEdit={(text) => onEdit(file.path, text)} onSave={() => onSave(file)} onDownload={() => onDownload(file)} />
    </EditorPane>
  );
}

export function FileAside(props: FileAsideProps) {
  const t = useTranslations("volumes.files.aside");
  if (props.mode === "split") return <SecondFile {...props} />;
  return (
    <>
      <AsideHeader title={t("title")} />
      <Details volume={props.volume} path={props.path} active={props.active} warning={props.warning} />
    </>
  );
}
