import { CircleCheck, CircleX, FolderInput, HardDrive, Network, Pencil, Plus, Server, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import type { TestResult } from "@/data/api";
import { useVolumes } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { validateRetention } from "@/domain/retention";
import type { BackupTarget, BackupTargetKind, HubSettings } from "@/domain/settings";
import { absolutePath, hostAddress, required } from "@/domain/validation";
import { TextField } from "@/shared/components/form-field";
import { RetentionEditor } from "@/shared/components/retention-editor";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { ToneBadge } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { draftId } from "@/shared/lib/ids";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { HelpLabel, HelpTarget, HelpTooltip } from "@/shared/help/help-tooltip";
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Spinner } from "@/shared/ui/spinner";

import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";

const KIND_ICON: Record<BackupTargetKind, React.ReactNode> = { local: <HardDrive />, mount: <FolderInput />, smb: <Network />, nfs: <Server /> };

function targetErrors(tg: BackupTarget) {
  const e: Partial<Record<"name" | "path" | "server" | "share" | "hostId", string>> = {};
  if (required(tg.name)) e.name = "required";
  if (absolutePath(tg.path)) e.path = absolutePath(tg.path);
  if ((tg.kind === "smb" || tg.kind === "nfs") && hostAddress(tg.server ?? "")) e.server = hostAddress(tg.server ?? "");
  if (tg.kind === "smb" && required(tg.share ?? "")) e.share = "required";
  if ((tg.kind === "local" || tg.kind === "mount") && !tg.hostId) e.hostId = "required";
  return e;
}

function TargetDialog({ target, onClose, onApply }: { target: BackupTarget | null; onClose: () => void; onApply: (t: BackupTarget) => void }) {
  const t = useTranslations("settings.backups");
  const tv = useTranslations("hosts.validation");
  const adapter = useAdapter();
  const hosts = useHostScope();
  const [draft, setDraft] = useState<BackupTarget>(() => target ?? { id: draftId("t"), name: "", kind: "smb", path: "/docklet", server: "", share: "", username: "" });
  const [password, setPassword] = useState("");
  const [tried, setTried] = useState(false);
  const [test, setTest] = useState<TestResult | "running" | null>(null);
  const e = targetErrors(draft);
  const err = (k: keyof typeof e) => (tried && e[k] ? tv(e[k] as "required") : undefined);
  const patch = (p: Partial<BackupTarget>) => {
    setDraft((d) => ({ ...d, ...p }));
    setTest(null);
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <form
          className="flex flex-col gap-4"
          onSubmit={(ev) => {
            ev.preventDefault();
            setTried(true);
            if (Object.keys(e).length) return;
            onApply({ ...draft, secretSet: draft.secretSet || password.length > 0 });
          }}
        >
          <DialogHeader>
            <DialogTitle>{target ? t("dialogEdit", { name: target.name }) : t("dialogAdd")}</DialogTitle>
            <DialogDescription>{t("dialogDescription")}</DialogDescription>
          </DialogHeader>
          <Field>
            <HelpLabel topic="settingsBackups.targetKind">{t("kind")}</HelpLabel>
            <SegmentedToggle value={draft.kind} onChange={(kind) => patch({ kind })} label={t("kind")} options={(["local", "mount", "smb", "nfs"] as const).map((k) => ({ value: k, label: t(`kindShort.${k}`), icon: KIND_ICON[k] }))} className="self-start" />
            <FieldDescription>{t(`kindHints.${draft.kind}`)}</FieldDescription>
          </Field>
          <TextField id="tg-name" label={t("name")} value={draft.name} onChange={(name) => patch({ name })} error={err("name")} placeholder="NAS (SMB)" />
          {(draft.kind === "local" || draft.kind === "mount") && (
            <Field data-invalid={err("hostId") ? true : undefined}>
              <FieldLabel htmlFor="tg-host">{t("host")}</FieldLabel>
              <Select value={draft.hostId ?? ""} onValueChange={(hostId) => patch({ hostId })}>
                <SelectTrigger id="tg-host" className="w-full">
                  <SelectValue placeholder="–" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {hosts.all.map((h) => (
                      <SelectItem key={h.id} value={h.id}>
                        {h.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          )}
          {(draft.kind === "smb" || draft.kind === "nfs") && (
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField id="tg-server" label={t("server")} value={draft.server ?? ""} onChange={(server) => patch({ server })} error={err("server")} mono placeholder="nas.example.com" />
              {draft.kind === "smb" && <TextField id="tg-share" label={t("share")} value={draft.share ?? ""} onChange={(share) => patch({ share })} error={err("share")} mono placeholder="backups" />}
            </div>
          )}
          <TextField id="tg-path" label={draft.kind === "nfs" ? t("exportPath") : draft.kind === "smb" ? t("subPath") : t("path")} value={draft.path} onChange={(path) => patch({ path })} error={err("path")} mono />
          {draft.kind === "smb" && (
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField id="tg-user" label={t("username")} value={draft.username ?? ""} onChange={(username) => patch({ username })} mono autoComplete="off" />
              <TextField id="tg-pass" type="password" label={t("password")} value={password} onChange={setPassword} description={draft.secretSet ? t("passwordKeep") : t("passwordDemo")} autoComplete="new-password" />
            </div>
          )}
          {(draft.kind === "smb" || draft.kind === "nfs") && <TextField id="tg-opts" label={t("options")} value={draft.options ?? ""} onChange={(options) => patch({ options })} mono placeholder={draft.kind === "smb" ? "vers=3.1.1" : "nfsvers=4.1"} />}
          <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3">
            <HelpTarget topic="settingsBackups.test">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={test === "running" || Object.keys(e).length > 0}
                onClick={async () => {
                  setTest("running");
                  setTest(await adapter.testBackupTarget({ ...draft, secretSet: draft.secretSet || password.length > 0 }));
                }}
              >
                {test === "running" && <Spinner data-icon="inline-start" />}
                {test === "running" ? t("testing") : t("test")}
              </Button>
            </HelpTarget>
            {test && test !== "running" && (
              <span role="status" className={test.ok ? "inline-flex items-center gap-1.5 text-xs text-success" : "inline-flex items-center gap-1.5 text-xs text-destructive"}>
                {test.ok ? <CircleCheck className="size-3.5" aria-hidden /> : <CircleX className="size-3.5" aria-hidden />}
                {test.ok ? t("testOk") : t("testFailed")}: {test.message}
              </span>
            )}
          </div>
          <p className="text-[11px] text-muted-foreground">{t("permissionsNote")}</p>
          <DialogFooter>
            <Button type="submit">{t("apply")}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function BackupTargetsSection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const tb = useTranslations("settings.backups");
  const dt = useAdapterText();
  const save = useSaveSettings();
  const volumes = useVolumes();
  const hosts = useHostScope();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.backups);
  const [editing, setEditing] = useState<BackupTarget | "new" | null>(null);
  if (!draft) return null;
  const used = new Set((volumes.data ?? []).map((v) => v.backup?.targetId).filter(Boolean));
  return (
    <SettingsForm
      title={t("sections.backups")}
      dirty={dirty}
      valid={!validateRetention(draft.defaultRetention)}
      onDiscard={reset}
      onSave={async () => {
        await save("backups", draft);
        commit(draft);
      }}
    >
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">{tb("targets")}</h3>
          <Button type="button" size="sm" variant="outline" onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            {tb("add")}
          </Button>
        </div>
        <ul className="grid gap-2 md:grid-cols-2">
          {draft.targets.map((tg) => (
            <li key={tg.id} className="flex items-start gap-3 rounded-xl border bg-card/40 p-3">
              <span className="mt-0.5 text-muted-foreground [&_svg]:size-4">{KIND_ICON[tg.kind]}</span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{tg.name}</span>
                  <ToneBadge tone="gray" className="h-5 px-1.5 text-[10.5px]">
                    {tb(`kinds.${tg.kind}`)}
                  </ToneBadge>
                  {used.has(tg.id) && <span className="text-[11px] text-muted-foreground">{tb("inUse")}</span>}
                </div>
                <div className="mt-0.5 truncate font-mono text-[11.5px] text-muted-foreground">
                  {tg.kind === "smb" ? `//${tg.server}/${tg.share}${tg.path}` : tg.kind === "nfs" ? `${tg.server}:${tg.path}` : `${hosts.byId.get(tg.hostId ?? "")?.name ?? tg.hostId}:${tg.path}`}
                </div>
                {tg.lastTest && (
                  <div className={tg.lastTest.ok ? "mt-1 text-[11px] text-success" : "mt-1 text-[11px] text-destructive"}>
                    {tg.lastTest.ok ? tb("testOk") : tb("testFailed")}: {dt(tg.lastTest.message)}
                  </div>
                )}
              </div>
              <div className="flex shrink-0 gap-1">
                <Button type="button" size="icon" variant="ghost" className="size-8" aria-label={`${tb("edit")}: ${tg.name}`} onClick={() => setEditing(tg)}>
                  <Pencil />
                </Button>
                <Button type="button" size="icon" variant="ghost" className="size-8 text-destructive hover:text-destructive" aria-label={`${tb("remove")}: ${tg.name}`} title={used.has(tg.id) ? tb("inUse") : undefined} onClick={() => patch({ targets: draft.targets.filter((x) => x.id !== tg.id) })}>
                  <Trash2 />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      </div>
      <RetentionEditor idPrefix="bk-default-ret" legend={tb("retention")} value={draft.defaultRetention} onChange={(defaultRetention) => patch({ defaultRetention })} className="max-w-2xl" />
      <p className="-mt-2 flex items-center gap-1 text-xs text-muted-foreground">
        {tb("retentionScope")}
        <HelpTooltip topic="settingsBackups.retention" />
      </p>
      {editing && (
        <TargetDialog
          target={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onApply={(tg) => {
            const exists = draft.targets.some((x) => x.id === tg.id);
            patch({ targets: exists ? draft.targets.map((x) => (x.id === tg.id ? tg : x)) : [...draft.targets, tg] });
            setEditing(null);
          }}
        />
      )}
    </SettingsForm>
  );
}
