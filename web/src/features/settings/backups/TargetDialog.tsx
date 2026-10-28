// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original target form with immutable locations and volatile credentials.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { backupErrorMessage } from "@/data/backup-error";
import type { BackupTarget, BackupTargetInput } from "@/domain/settings";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

export function TargetDialog({
  target,
  onClose,
  saved,
}: {
  target?: BackupTarget;
  onClose: () => void;
  saved: () => void;
}) {
  const adapter = useAdapter(),
    t = useTranslations("settings.backups"),
    all = useTranslations(),
    hosts = useHostScope();
  const [draft, setDraft] = useState<BackupTargetInput>(() =>
    target
      ? {
          name: target.name,
          kind: target.kind,
          path: target.path,
          hostId: target.hostId,
          server: target.server,
          share: target.share,
          username: target.username,
          options: target.options,
        }
      : { name: "", kind: "smb", path: "", server: "", share: "", username: "", options: "vers=3.1.1" },
  );
  const [secret, setSecret] = useState(""),
    [key, setKey] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>();
  const patch = (value: Partial<BackupTargetInput>) => setDraft((old) => ({ ...old, ...value }));
  const remote = draft.kind === "smb" || draft.kind === "nfs";
  const invalid =
    !draft.name.trim() ||
    (remote
      ? !draft.server || !draft.share || draft.path.startsWith("/") || draft.path.split("/").includes("..")
      : !draft.hostId || !draft.path.startsWith("/")) ||
    (draft.kind === "smb" &&
      ((/[,:=]/.test(draft.server ?? "") && (draft.server ?? "").includes(":")) ||
        /[,=]/.test(`${draft.username ?? ""}${secret}`)));
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <form
          className="space-y-4"
          onSubmit={async (event) => {
            event.preventDefault();
            setBusy(true);
            setError(undefined);
            try {
              const input = {
                ...draft,
                ...(secret ? { secret } : {}),
                ...(!target && key ? { recoveryKey: key } : {}),
              };
              if (target) await adapter.updateBackupTarget(target.id, input);
              else await adapter.createBackupTarget(input);
              setSecret("");
              setKey("");
              saved();
              onClose();
            } catch (cause) {
              setError(backupErrorMessage(cause, all));
            } finally {
              setBusy(false);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{target ? t("dialogEdit", { name: target.name }) : t("dialogAdd")}</DialogTitle>
            <DialogDescription>{t("dialogDescription")}</DialogDescription>
          </DialogHeader>
          <TextField id="tg-name" label={t("name")} value={draft.name} onChange={(name) => patch({ name })} />
          <fieldset disabled={!!target || busy} className="space-y-3">
            <Field>
              <FieldLabel htmlFor="tg-kind">{t("kind")}</FieldLabel>
              <Select
                value={draft.kind}
                onValueChange={(kind: BackupTargetInput["kind"]) =>
                  setDraft({
                    name: draft.name,
                    kind,
                    path: "",
                    ...(kind === "smb" || kind === "nfs"
                      ? { server: "", share: "", options: kind === "nfs" ? "hard" : "vers=3.1.1" }
                      : { hostId: hosts.all[0]?.id }),
                  })
                }
              >
                <SelectTrigger id="tg-kind">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["local", "mount", "smb", "nfs"] as const).map((kind) => (
                    <SelectItem key={kind} value={kind}>
                      {t(`kinds.${kind}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {!remote ? (
              <Field>
                <FieldLabel htmlFor="tg-host">{t("host")}</FieldLabel>
                <Select value={draft.hostId ?? ""} onValueChange={(hostId) => patch({ hostId })}>
                  <SelectTrigger id="tg-host">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {hosts.all.map((host) => (
                      <SelectItem key={host.id} value={host.id}>
                        {host.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            ) : (
              <>
                <TextField
                  id="tg-server"
                  label={t("server")}
                  value={draft.server ?? ""}
                  onChange={(server) => patch({ server })}
                  description={draft.kind === "smb" ? t("smbHostHint") : undefined}
                />
                <TextField
                  id="tg-share"
                  label={draft.kind === "nfs" ? t("exportPath") : t("share")}
                  value={draft.share ?? ""}
                  onChange={(share) => patch({ share })}
                />
              </>
            )}
            <TextField
              id="tg-path"
              label={remote ? t("subPath") : t("path")}
              value={draft.path}
              onChange={(path) => patch({ path })}
            />
          </fieldset>
          {target && <p className="text-xs text-muted-foreground">{t("immutableLocation")}</p>}
          {draft.kind === "smb" && (
            <>
              <TextField
                id="tg-user"
                label={t("username")}
                value={draft.username ?? ""}
                onChange={(username) => patch({ username })}
              />
              <TextField
                id="tg-secret"
                type="password"
                label={t("password")}
                value={secret}
                onChange={setSecret}
                autoComplete="new-password"
                description={target?.hasSecret ? t("passwordKeep") : t("credentialHint")}
              />
              <p className="text-xs">{t("credentialHint")}</p>
            </>
          )}
          {remote && (
            <TextField
              id="tg-options"
              label={t("options")}
              value={draft.options ?? ""}
              onChange={(options) => patch({ options })}
              description={t("optionsHint")}
            />
          )}
          {!target && (
            <TextField
              id="tg-key"
              type="password"
              label={t("existingRecoveryKey")}
              value={key}
              onChange={setKey}
              autoComplete="off"
              description={t("existingKeyHint")}
            />
          )}
          <p className="text-xs text-muted-foreground">{t("permissionsNote")}</p>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={busy} onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={busy || !!invalid}>
              {t("apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
