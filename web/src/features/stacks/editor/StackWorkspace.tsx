// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/features/stacks/editor/StackWorkspace.tsx
// Revision: 8dc34a900c94d090a1c5ae85199143f6816fe69d3e3689e6fd22806a54d8505f
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: M2 production shell: disconnected error handling, development-only demo boundary, neutral product text or responsive layout. Historical import recorded in web/PROVENANCE.md. M7 stack authoring: hub create/save/deploy, docker run conversion and production import boundary. #182 workspace editing, ordering and lifecycle. Simple-user management, assigned-stack access and mode-aware self-service.

import { useIsUser } from "@/data/use-principal";
import { AlertTriangle, CheckCircle2, ChevronDown, Code2, FlaskConical, Import, Play, Save, Terminal, Wand2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useBlocker, useNavigate } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { queueDemoFailure, type DemoFailure } from "@/runtime";
import { isRunBusy } from "@/data/run-gate";
import { StackCreatedError } from "@/data/adapter";
import { WorkspaceConflictError, WorkspaceError } from "@/data/http/workspace";
import { workspaceErrorKey } from "@/features/volumes/files/workspace-error";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { ProjectFileActions } from "./ProjectFileActions";
import { ProjectFileEditor } from "./ProjectFileEditor";
import { useProjectWorkspace } from "./use-project-workspace";
import { AdapterOutcomeError } from "@/data/capability";
import { useRuntimeError } from "@/features/activity/use-runtime-error";
import { useCapability } from "@/data/use-capability";
import { useAdapter } from "@/data/use-adapter";
import { ops, uniqueKey, type Mutator } from "@/domain/compose/ops";
import type { Host, Stack } from "@/domain/model";
import { sameStackLabels } from "@/domain/stack-labels";
import { Alert, AlertDescription } from "@/shared/ui/alert";
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
import type { Declarations } from "./hub-conversion";

export type EditorMode = "easy" | "advanced";

interface StackWorkspaceProps {
  draft: ComposeDraft;
  stack: (Stack & { filesUnavailable?: boolean }) | null;
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
export function StackWorkspace(props: StackWorkspaceProps) {
  return <StackWorkspaceBody key={props.stack?.id ?? `new:${props.draft.hostId}`} {...props} />;
}

function StackWorkspaceBody({ draft, stack, hosts, labelSuggestions, mode, onMode }: StackWorkspaceProps) {
  const t = useTranslations("editor");
  const tv = useTranslations("volumes.files");
  const [file, setFile] = useState<WorkspaceFile>("compose");
  const extra = useProjectWorkspace(stack?.id ?? null, file);
  const extraCapability = useCapability("updateProjectWorkspace", { kind: "stack", id: stack?.id ?? "" });
  const extraDownload = useCapability("downloadProjectFile", { kind: "stack", id: stack?.id ?? "" });
  const actionLock = useRef(false);
  const proceeding = useRef(false);
  const [partialSave, setPartialSave] = useState(false);
  const dirty = draft.dirty || extra.dirty;
  const blocker = useBlocker(({ currentLocation, nextLocation }) => !proceeding.current && dirty && (currentLocation.pathname !== nextLocation.pathname || new URLSearchParams(nextLocation.search).get("tab") !== "editor"));
  const adapter = useAdapter();
  const navigate = useNavigate();
  const text = useEditorText();
  const runtimeError = useRuntimeError();
  // Local completion effects (navigation) belong to this workspace; the hub work continues regardless.
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const user = useIsUser();
  const deployCapability = useCapability(user && stack ? "saveStackFiles" : "deployStack", stack ? { kind: "stack", id: stack.id } : { kind: "host", id: draft.hostId });
  const saveCapability = useCapability("saveStackFiles", { kind: "stack", id: stack?.id ?? "" });
  const labelCapability = useCapability("setStackLabels", { kind: "stack", id: stack?.id ?? "" });
  // Save writes labels too when they differ; both descriptors must allow it.
  const labelsChanged = !!stack && !sameStackLabels(draft.labels, stack.labels);
  const filesBlocked = stack?.filesUnavailable ? t("filesStale") : undefined;
  const extraBlocked = extra.dirty && (extra.conflicted || extra.tree.isError || !extraCapability.available) ? extraCapability.reason ?? tv("workspaceErrors.conflict") : undefined;
  const saveBlocked = filesBlocked ?? extraBlocked ?? (!saveCapability.available ? saveCapability.reason : labelsChanged && !labelCapability.available ? labelCapability.reason : undefined);
  const deployBlocked = filesBlocked ?? extraBlocked ?? (!deployCapability.available ? deployCapability.reason : undefined);
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

  /** Shows authoring failures; the draft always stays for a later retry. */
  const report = (e: unknown) => {
    if (!mounted.current) return;
    if (e instanceof WorkspaceConflictError || e instanceof WorkspaceError) { toast.error(tv(`workspaceErrors.${workspaceErrorKey(e)}`)); return; }
    const code = e instanceof AdapterOutcomeError && e.outcome.kind === "error" ? e.outcome.code : "";
    if (code === "stack.host-immutable") toast.error(t("hostImmutable"));
    else if (code.startsWith("stack.") && code.endsWith("-unknown")) toast.error(t("writeUnknown"));
    else if (!runtimeError(e)) throw e;
  };

  /** docker run results arrive as one atomic edit including their top-level declarations. */
  const importRun = (base: string, body: Record<string, unknown>, env: string, declarations: Declarations) => {
    const key = uniqueKey(model?.services.map((s) => s.key) ?? [], base);
    const err = draft.edit(ops.importService(key, body, declarations));
    if (err) { toast.error(text.editError(err)); return; }
    if (env.trim()) draft.setEnv(`${draft.env.trimEnd() ? `${draft.env.trimEnd()}\n` : ""}${env.trim()}\n`);
    toast.success(t("added", { name: key }));
    setWizard({ step: 2, service: key, section: "general" });
  };

  /** Compose/Env first, extra files second, labels last; stop on any failure. */
  const persist = async () => {
    if (!stack) return;
    let wroteCompose = false;
    try {
      if (draft.dirty) {
        await adapter.saveStackFiles(stack.id, draft.compose, draft.env);
        wroteCompose = true;
      }
      const extraSaved = await extra.save();
      if (!sameStackLabels(draft.labels, stack.labels)) await adapter.setStackLabels(stack.id, draft.labels);
      if (!mounted.current) return;
      draft.markSaved();
      setPartialSave(false);
      return extraSaved;
    } catch (error) {
      if (mounted.current) setPartialSave(wroteCompose);
      throw error;
    }
  };

  const save = async () => {
    if (!stack || saveBlocked || !dirty || actionLock.current) return;
    actionLock.current = true; setSaving(true);
    try {
      const complete = await persist();
      if (mounted.current) toast.success(t(complete === false ? "extraFiles.newerDraft" : "saved"));
    } catch (error) { if (mounted.current) report(error); }
    finally { actionLock.current = false; if (mounted.current) setSaving(false); }
  };

  const deploy = async (failAt?: DemoFailure["at"]) => {
    if (!model || deployBlocked || actionLock.current) return;
    actionLock.current = true; setSaving(true);
    let stackId: string;
    try {
      if (extra.dirty && await persist() === false) { toast.info(t("extraFiles.newerDraft")); return; }
      if (!mounted.current) return;
      if (failAt) queueDemoFailure(adapter, { operation: "deployStack", at: failAt });
      ({ stackId } = await adapter.deployStack({ stackId: stack?.id ?? null, hostId: draft.hostId, name: model.name, compose: draft.compose, env: draft.env, labels: draft.labels }));
    } catch (e) {
      if (!mounted.current) return;
      // A busy stack keeps the draft; the gate notice names the running action.
      if (isRunBusy(e)) return;
      if (e instanceof StackCreatedError) {
        // The files are on the hub; retrying from the new-stack draft would collide by name.
        draft.markSaved();
        if (e.admission === "unknown") toast.error(t("createdDeployUnknown"));
        else toast.error(t("createdNotDeployed"));
        if (mounted.current) { proceeding.current = true; navigate(`/stacks/${e.stackId}?tab=editor`, { replace: true }); }
        return;
      }
      if (mounted.current) report(e);
      return;
    } finally { actionLock.current = false; if (mounted.current) setSaving(false); }
    if (!mounted.current) return;
    draft.markSaved();
    if (isNew && mounted.current) { proceeding.current = true; navigate(`/stacks/${stackId}?tab=deployments`, { replace: true }); }
  };

  const deployButton = (
    <div className="flex">
      <HelpTarget topic={isNew ? "action.deploy" : "action.saveDeploy"}>
        <Button size="sm" className="rounded-r-none" disabled={!model || errors > 0 || saving || extra.saving || !!deployBlocked} title={deployBlocked} onClick={() => void deploy()}>
          <Play data-icon="inline-start" />
          {isNew ? t("deploy") : t("saveDeploy")}
        </Button>
      </HelpTarget>
      {import.meta.env.DEV && <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" className="rounded-l-none border-l border-primary-foreground/20 px-2" disabled={!model || errors > 0 || saving || extra.saving || !!deployBlocked} title={deployBlocked} aria-label={t("deployMenu")}>
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
      </DropdownMenu>}
    </div>
  );

  return (
    <WorkspaceFrame>
      {partialSave && <Alert><AlertTriangle /><AlertDescription>{t("extraFiles.partialSave")}</AlertDescription></Alert>}
      {filesBlocked && <p role="alert" className="rounded-lg border border-warning/30 bg-warning/8 px-3 py-2 text-sm text-warning">{filesBlocked}</p>}
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
            {adapter.kind === "demo" && (
              <HelpTarget topic="stack.importSource">
                <Button size="sm" variant="outline" disabled={!model} onClick={() => setDialog("import")}>
                  <Import data-icon="inline-start" /> {t("importSource")}
                </Button>
              </HelpTarget>
            )}
          </>
        )}
        <span className="flex-1" />
        <ToneBadge tone={!model || errors ? "err" : warns ? "warn" : "ok"} className="h-auto max-w-full flex-wrap whitespace-normal">
          {model ? <CheckCircle2 /> : <AlertTriangle />}
          {model ? t("valid") : t("invalid")}
          {model && <span className="text-(--tone)/80">· {t("issues", { errors, warns })}</span>}
        </ToneBadge>
        {!isNew && (
          <Button size="sm" variant="outline" disabled={!dirty || saving || extra.saving || !!saveBlocked} title={saveBlocked} onClick={() => void save()}>
            <Save data-icon="inline-start" /> {t("save")}
          </Button>
        )}
        {deployButton}
        {mode === "advanced" && <AsideToggle value={aside} onChange={(m) => setPreference("workspaceAside", m)} />}
      </div>

      {mode === "advanced" && extra.supported && stack && <>
        <ProjectFileActions workspace={extra} id={stack.id} busy={saving} report={report} />
        {extra.tree.isError && <p role="alert">{tv(`workspaceErrors.${workspaceErrorKey(extra.tree.error)}`)}</p>}
        {extra.tree.data?.fileTreeTruncated && <p role="status">{t("extraFiles.truncated")}</p>}
      </>}
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
              extra.setActive(null);
              extra.setSelected(null);
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
          aside={aside === "off" || extra.active ? undefined : <ComposeAside mode={aside} draft={draft} file={file} hostId={draft.hostId} stackId={stack?.id ?? null} hostProjectDir={host?.projectDir} onFile={setFile} />}
          sidebar={
            <ComposeFileTree
              projectName={model?.name ?? stack?.id ?? ""}
              projectDir={`${host?.projectDir ?? "/opt/stacks"}/${model?.name || "<stackname>"}`}
              hostProjectDir={host?.projectDir}
              hostId={draft.hostId}
              stackId={stack?.id ?? null}
              active={extra.active ? null : file}
              activeExtra={extra.selected ?? extra.active}
              extraFiles={extra.nodes}
              onOpenExtra={(path, directory) => { extra.setSelected(path); if (!directory) extra.open(path); }}
              composeDirty={draft.composeDirty}
              envDirty={draft.envDirty}
              model={model}
              onOpen={(next) => { extra.setActive(null); extra.setSelected(null); setFile(next); }}
            />
          }
        >
          {extra.active ? <ProjectFileEditor workspace={extra} canDownload={extraDownload.available} readOnly={!extraCapability.available} onSave={() => void save()} onDownload={() => {
            if (!stack || !adapter.downloadProjectFile) return;
            const path = extra.active!;
            void adapter.downloadProjectFile(stack.id, path).then((blob) => {
              if (!mounted.current) return;
              const url = URL.createObjectURL(blob); const link = document.createElement("a"); link.href = url; link.download = path.split("/").at(-1)!; link.click(); URL.revokeObjectURL(url);
            }).catch(report);
          }} /> : <AdvancedEditor draft={draft} file={file} onFile={setFile} canReset={!isNew} onSave={!isNew && dirty && !saving ? () => void save() : undefined} />}
        </Workspace>
      )}
      {mode === "advanced" && (
        <p id="raw-help" className="text-xs text-muted-foreground">
          {t("raw.help")}
        </p>
      )}

      <ConfirmDialog open={blocker.state === "blocked"} onOpenChange={(on) => { if (!on && blocker.state === "blocked" && !proceeding.current) blocker.reset(); }} title={t("extraFiles.leaveTitle")} description={t("extraFiles.leaveBody")} confirmLabel={t("extraFiles.leave")} destructive onConfirm={() => { if (blocker.state === "blocked") { proceeding.current = true; blocker.proceed(); } }} />
      <DockerRunDialog open={dialog === "run"} onOpenChange={(o) => setDialog(o ? "run" : null)} existing={model?.services.map((s) => s.key) ?? []} hostId={draft.hostId}
        onAdd={(key, body, env, declarations) => importRun(key, body, env, declarations)} />
      <ImportSourceDialog open={dialog === "import"} onOpenChange={(o) => setDialog(o ? "import" : null)} onApply={(r) => addServices(r.services)} />
    </WorkspaceFrame>
  );
}
