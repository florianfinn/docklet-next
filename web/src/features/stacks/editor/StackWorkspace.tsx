import { AlertTriangle, CheckCircle2, ChevronDown, Code2, FlaskConical, Import, Play, Save, Terminal, Wand2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { FailAt } from "@/data/adapter";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import { ops, uniqueKey, type Mutator } from "@/domain/compose/ops";
import type { Host, Stack } from "@/domain/model";
import { sameStackLabels } from "@/domain/stack-labels";
import { ToneBadge } from "@/shared/components/tone";
import { setPreference, usePreference } from "@/shared/preferences/preferences-store";
import { AsideToggle, Workspace, WorkspaceFrame } from "@/shared/workspace/workspace";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";
import { HelpTarget, HelpTooltip } from "@/shared/help/help-tooltip";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";

import { AdvancedEditor } from "./AdvancedEditor";
import { ComposeAside } from "./ComposeAside";
import { DockerRunDialog } from "./DockerRunDialog";
import { EasyWizard, type WizardState } from "./EasyWizard";
import { useEditorText } from "./editor-text";
import { ImportSourceDialog } from "./ImportSourceDialog";
import type { ComposeDraft } from "./use-compose-draft";
import { ComposeFileTree, type WorkspaceFile } from "./ComposeFileTree";

export type EditorMode = "easy" | "advanced";

interface StackWorkspaceProps {
  draft: ComposeDraft;
  stack: Stack | null;
  hosts: Host[];
  labelSuggestions: string[];
  mode: EditorMode;
  onMode: (m: EditorMode) => void;
}

/**
 * Stack editor with one draft: Easy replaces the whole editor area with a
 * full-width wizard (no file tree, no raw preview); Advanced is the shared
 * file workspace with tree, CodeMirror and an optional aside on wide screens.
 */
export function StackWorkspace({ draft, stack, hosts, labelSuggestions, mode, onMode }: StackWorkspaceProps) {
  const t = useTranslations("editor");
  const adapter = useAdapter();
  const navigate = useNavigate();
  const text = useEditorText();
  const [file, setFile] = useState<WorkspaceFile>("compose");
  const [wizard, setWizard] = useState<WizardState>({ step: stack ? 2 : 1, service: null, section: "general" });
  const aside = usePreference("workspaceAside");
  const [dialog, setDialog] = useState<"run" | "import" | null>(null);
  const [saving, setSaving] = useState(false);
  const isNew = !stack;
  const model = draft.parsed.ok ? draft.parsed.model : null;
  const errors = draft.issues.filter((i) => i.level === "error").length;
  const warns = draft.issues.length - errors;
  const host = hosts.find((h) => h.id === draft.hostId);

  const edit = (fn: Mutator) => {
    const err = draft.edit(fn);
    if (err) toast.error(text.editError(err));
  };

  const addServices = (list: { key: string; body: Record<string, unknown> }[]) => {
    const existing = model?.services.map((s) => s.key) ?? [];
    let last = "";
    for (const s of list) {
      const key = uniqueKey(existing, s.key);
      existing.push(key);
      edit(ops.addService(key, s.body));
      last = key;
      toast.success(t("added", { name: key }));
    }
    setWizard({ step: 2, service: last || null, section: "general" });
  };

  const save = async () => {
    if (!stack) return;
    setSaving(true);
    try {
      await adapter.saveStackFiles(stack.id, draft.compose, draft.env);
      if (!sameStackLabels(draft.labels, stack.labels)) await adapter.setStackLabels(stack.id, draft.labels);
      draft.markSaved();
      toast.success(t("saved"));
    } finally {
      setSaving(false);
    }
  };

  const deploy = async (failAt?: FailAt) => {
    if (!model) return;
    let stackId: string;
    try {
      ({ stackId } = await adapter.deployStack({ stackId: stack?.id ?? null, hostId: draft.hostId, name: model.name, compose: draft.compose, env: draft.env, labels: draft.labels, failAt }));
    } catch (e) {
      // A busy stack keeps the draft; the gate notice names the running action.
      if (isRunBusy(e)) return;
      throw e;
    }
    draft.markSaved();
    if (isNew) navigate(`/stacks/${stackId}?tab=deployments`, { replace: true });
  };

  const deployButton = (
    <div className="flex">
      <HelpTarget topic={isNew ? "action.deploy" : "action.saveDeploy"}>
        <Button size="sm" className="rounded-r-none" disabled={!model || errors > 0} onClick={() => void deploy()}>
          <Play data-icon="inline-start" />
          {isNew ? t("deploy") : t("saveDeploy")}
        </Button>
      </HelpTarget>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" className="rounded-l-none border-l border-primary-foreground/20 px-2" disabled={!model || errors > 0} aria-label={t("deployMenu")}>
            <ChevronDown />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuGroup>
            <DropdownMenuItem onSelect={() => void deploy("start")}>
              <FlaskConical /> {t("deployFail")}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );

  return (
    <WorkspaceFrame>
      <div className="flex flex-wrap items-center gap-2">
        <SegmentedToggle
          value={mode}
          onChange={onMode}
          label={t("mode.label")}
          options={[
            { value: "easy", label: t("mode.easy"), icon: <Wand2 />, hint: t("mode.easyHint") },
            { value: "advanced", label: t("mode.advanced"), icon: <Code2 />, hint: t("mode.advancedHint") },
          ]}
        />
        <HelpTooltip topic="stack.mode" />
        {mode === "easy" && (
          <>
            <HelpTarget topic="stack.importRun">
              <Button size="sm" variant="outline" disabled={!model} onClick={() => setDialog("run")}>
                <Terminal data-icon="inline-start" /> {t("importRun")}
              </Button>
            </HelpTarget>
            <HelpTarget topic="stack.importSource">
              <Button size="sm" variant="outline" disabled={!model} onClick={() => setDialog("import")}>
                <Import data-icon="inline-start" /> {t("importSource")}
              </Button>
            </HelpTarget>
          </>
        )}
        <span className="flex-1" />
        <ToneBadge tone={!model || errors ? "err" : warns ? "warn" : "ok"}>
          {model ? <CheckCircle2 /> : <AlertTriangle />}
          {model ? t("valid") : t("invalid")}
          {model && <span className="text-(--tone)/80">· {t("issues", { errors, warns })}</span>}
        </ToneBadge>
        {!isNew && (
          <Button size="sm" variant="outline" disabled={!draft.dirty || saving} onClick={() => void save()}>
            <Save data-icon="inline-start" /> {t("save")}
          </Button>
        )}
        {deployButton}
        {mode === "advanced" && <AsideToggle value={aside} onChange={(m) => setPreference("workspaceAside", m)} />}
      </div>

      {mode === "easy" ? (
        // The form keeps a readable width, also on 21:9 screens; the editor toolbar above stays full width.
        <section aria-label={t("wizard")} data-slot="easy-wizard" data-glow className="w-full max-w-[100rem] rounded-2xl border border-primary/20 bg-card p-4 md:p-6">
          <p className="mb-4 text-xs text-muted-foreground">{t("easyNote")}</p>
          <EasyWizard
            draft={draft}
            wizard={wizard}
            onWizard={setWizard}
            hosts={hosts}
            isNew={isNew}
            labelSuggestions={labelSuggestions}
            edit={edit}
            onEditError={(m) => toast.error(m)}
            onOpenRaw={() => {
              setFile("compose");
              onMode("advanced");
            }}
            deploy={deployButton}
          />
        </section>
      ) : (
        <Workspace
          label={t("workspace")}
          asideLabel={t("aside.title")}
          asideKind={aside === "split" ? "split" : "details"}
          aside={aside === "off" ? undefined : <ComposeAside mode={aside} draft={draft} file={file} hostId={draft.hostId} stackId={stack?.id ?? null} hostProjectDir={host?.projectDir} onFile={setFile} />}
          sidebar={
            <ComposeFileTree
              projectName={model?.name ?? stack?.id ?? ""}
              projectDir={`${host?.projectDir ?? "/opt/stacks"}/${model?.name || "<stackname>"}`}
              hostProjectDir={host?.projectDir}
              hostId={draft.hostId}
              stackId={stack?.id ?? null}
              active={file}
              composeDirty={draft.composeDirty}
              envDirty={draft.envDirty}
              model={model}
              onOpen={setFile}
            />
          }
        >
          <AdvancedEditor draft={draft} file={file} onFile={setFile} canReset={!isNew} onSave={!isNew && draft.dirty && !saving ? () => void save() : undefined} />
        </Workspace>
      )}
      {mode === "advanced" && (
        <p id="raw-help" className="text-xs text-muted-foreground">
          {t("raw.help")}
        </p>
      )}

      <DockerRunDialog open={dialog === "run"} onOpenChange={(o) => setDialog(o ? "run" : null)} existing={model?.services.map((s) => s.key) ?? []} onAdd={(key, body) => addServices([{ key, body }])} />
      <ImportSourceDialog open={dialog === "import"} onOpenChange={(o) => setDialog(o ? "import" : null)} onApply={(r) => addServices(r.services)} />
    </WorkspaceFrame>
  );
}
