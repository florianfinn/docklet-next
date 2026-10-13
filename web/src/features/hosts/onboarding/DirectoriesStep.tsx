import { ChevronDown, FolderCog, FolderTree, Info, ServerCog } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { SYSTEM_LABEL } from "@/domain/system";
import { defaultInstallDir, isInside, normalizePath } from "@/domain/validation";
import { TextField } from "@/shared/components/form-field";
import { cn } from "@/shared/lib/utils";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Switch } from "@/shared/ui/switch";

import { patchDraft, usesRootIds, type DraftErrors, type HostDraft } from "./onboarding-draft";
import { useFieldError } from "./use-field-error";

function Tree({ title, icon, tone, lines }: { title: string; icon: React.ReactNode; tone: string; lines: string }) {
  return (
    <div className="min-w-0">
      <div className={cn("mb-1.5 flex items-center gap-1.5 text-xs font-semibold [&_svg]:size-3.5", tone)}>
        {icon}
        {title}
      </div>
      <pre className="overflow-x-auto font-mono text-[11.5px] leading-relaxed text-muted-foreground">{lines}</pre>
    </div>
  );
}

/** Folder layout as the installer will create it; the agent may live in its own folder inside the project directory. */
function DirDiagram({ projectDir, installDir }: { projectDir: string; installDir: string }) {
  const t = useTranslations("hosts.onboarding");
  const p = normalizePath(projectDir || "/home/docker");
  const i = normalizePath(installDir || defaultInstallDir(p));
  const inside = isInside(i, p) && i !== p;
  const agentFiles = "├─ compose.yaml\n├─ .env\n└─ data/";
  return (
    <div className="grid gap-3 rounded-xl border bg-card/40 p-3.5 sm:grid-cols-2" aria-label={t("dirsTitle")}>
      {inside ? (
        <Tree
          title={t("projectDir")}
          icon={<FolderTree />}
          tone="text-primary-text"
          lines={`${p}/\n├─ media/compose.yaml\n├─ cloud/compose.yaml\n└─ ${i.slice(p.length + 1)}/   ← ${t("agentSystem")}\n   ${agentFiles.replaceAll("\n", "\n   ")}`}
        />
      ) : (
        <>
          <Tree title={t("projectDir")} icon={<FolderTree />} tone="text-primary-text" lines={`${p}/\n├─ media/compose.yaml\n├─ cloud/compose.yaml\n└─ …`} />
          <Tree title={t("installDir")} icon={<FolderCog />} tone="text-info" lines={`${i}/\n${agentFiles}`} />
        </>
      )}
      <p className="text-xs text-muted-foreground sm:col-span-2">{inside ? t("dirsExplainInside", { label: `${SYSTEM_LABEL}=agent` }) : t("dirsExplain")}</p>
    </div>
  );
}

/** What gets checked, where and when: locally by the installer on the target host, reported after connecting. */
function LocalCheckNote() {
  const t = useTranslations("hosts.onboarding.localCheck");
  return (
    <section aria-labelledby="local-check-title" className="flex gap-3 rounded-xl border border-info/30 bg-info/6 p-3.5">
      <ServerCog className="mt-0.5 size-4 shrink-0 text-info" aria-hidden />
      <div className="flex min-w-0 flex-col gap-1.5 text-xs">
        <h3 id="local-check-title" className="text-sm font-medium text-foreground">
          {t("title")}
        </h3>
        <p className="text-muted-foreground">{t("lead")}</p>
        <ul className="list-disc pl-4 text-muted-foreground">
          <li>{t("exists")}</li>
          <li>{t("owner")}</li>
          <li>{t("socket")}</li>
        </ul>
        <p className="text-muted-foreground">{t("report")}</p>
      </div>
    </section>
  );
}

export function DirectoriesStep({ draft, errors, showErrors, locked }: { draft: HostDraft; errors: DraftErrors; showErrors: boolean; locked: boolean }) {
  const t = useTranslations("hosts.onboarding");
  const err = useFieldError(errors, showErrors);
  const [open, setOpen] = useState(draft.manualIds || !draft.detectSocket);
  const example = defaultInstallDir(draft.projectDir || "/home/docker");

  // The default agent folder follows the project directory until the user picks another one.
  const setProjectDir = (projectDir: string) =>
    patchDraft(normalizePath(draft.installDir) === defaultInstallDir(draft.projectDir) ? { projectDir, installDir: defaultInstallDir(projectDir) } : { projectDir });

  return (
    <FieldGroup>
      <fieldset disabled={locked} className="contents">
        <TextField
          id="host-project-dir"
          label={t("projectDir")}
          value={draft.projectDir}
          onChange={setProjectDir}
          description={t("projectDirHint", { example: `${normalizePath(draft.projectDir || "/home/docker")}/media/compose.yaml` })}
          error={err("projectDir")}
          mono
        />
        <TextField
          id="host-install-dir"
          label={t("installDir")}
          value={draft.installDir}
          onChange={(installDir) => patchDraft({ installDir })}
          description={t("installDirHint", { example })}
          error={err("installDir")}
          mono
        />
      </fieldset>
      <DirDiagram projectDir={draft.projectDir} installDir={draft.installDir} />
      <LocalCheckNote />
      <Collapsible open={open} onOpenChange={setOpen} className="rounded-xl border">
        <CollapsibleTrigger className="flex w-full items-center gap-2 rounded-xl px-3.5 py-3 text-left text-sm font-medium outline-none hover:bg-muted/30 focus-visible:ring-[3px] focus-visible:ring-ring/50">
          <ChevronDown className={cn("size-4 transition-transform duration-(--dur)", !open && "-rotate-90")} aria-hidden />
          {t("advanced")}
          <span className="ml-auto text-xs font-normal text-muted-foreground">{t("advancedHint")}</span>
        </CollapsibleTrigger>
        <CollapsibleContent className="data-[state=open]:animate-in data-[state=open]:fade-in-0">
          <fieldset disabled={locked} className="flex flex-col gap-5 border-t p-3.5">
            <p className="flex items-start gap-2 text-xs text-muted-foreground">
              <Info className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              {t("noChown")}
            </p>
            <div className="flex flex-col gap-3">
              <Field orientation="horizontal">
                <Switch id="host-manual-ids" checked={draft.manualIds} onCheckedChange={(manualIds) => patchDraft({ manualIds })} />
                <FieldContent>
                  <FieldLabel htmlFor="host-manual-ids">{t("manualIds")}</FieldLabel>
                  <FieldDescription>{t("manualIdsHint")}</FieldDescription>
                </FieldContent>
              </Field>
              {draft.manualIds && (
                <div className="grid grid-cols-2 gap-3 sm:max-w-xs">
                  <TextField id="host-puid" label={t("puid")} value={draft.puid} onChange={(puid) => patchDraft({ puid })} error={err("puid")} mono />
                  <TextField id="host-pgid" label={t("pgid")} value={draft.pgid} onChange={(pgid) => patchDraft({ pgid })} error={err("pgid")} mono />
                </div>
              )}
              {usesRootIds(draft) && <p className="text-xs text-warning">{t("rootWarning")}</p>}
            </div>
            <div className="flex flex-col gap-3 border-t pt-4">
              <div>
                <div className="text-sm font-medium">{t("socketTitle")}</div>
                <p className="text-xs text-muted-foreground">{t("socketExplain")}</p>
              </div>
              <Field orientation="horizontal">
                <Switch id="host-detect-socket" checked={draft.detectSocket} onCheckedChange={(detectSocket) => patchDraft({ detectSocket })} />
                <FieldLabel htmlFor="host-detect-socket">{t("detectSocket")}</FieldLabel>
              </Field>
              {!draft.detectSocket && (
                <TextField id="host-socket-gid" label={t("socketGid")} value={draft.socketGid} onChange={(socketGid) => patchDraft({ socketGid })} error={err("socketGid")} mono className="sm:max-w-40" />
              )}
            </div>
          </fieldset>
        </CollapsibleContent>
      </Collapsible>
    </FieldGroup>
  );
}
