import { isHostAvailable } from "@/domain/hosts";
// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original live targets CRUD and per-host delivery/test results. M14 host maintenance and availability.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useBackupOverviews, useBackupTargets } from "@/data/backup-queries";
import { BackupError, backupCodeKey, backupErrorMessage } from "@/data/backup-error";
import { useAdapter } from "@/data/use-adapter";
import type { BackupTarget } from "@/domain/settings";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { BackupOperation } from "@/features/volumes/backups/BackupOperation";
import { RecoveryKeyDialog } from "../backups/RecoveryKeyDialog";
import { RepositoryCard } from "../backups/RepositoryCard";
import { TargetDialog } from "../backups/TargetDialog";

export function BackupTargetsSection() {
  return <CapabilityGate operation="listBackupTargets"><Targets /></CapabilityGate>;
}
function Targets() {
  const adapter = useAdapter(),
    t = useTranslations("settings.backups"),
    all = useTranslations(),
    hosts = useHostScope(),
    targets = useBackupTargets(),
    overviews = useBackupOverviews();
  const [editing, setEditing] = useState<BackupTarget | "new">(),
    [remove, setRemove] = useState<BackupTarget>(),
    [key, setKey] = useState<{ id: string; hostId?: string; name: string }>(),
    [error, setError] = useState<string>();
  return <section className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h2 className="text-lg font-semibold">{t("targets")}</h2>
      <Button onClick={() => setEditing("new")}>{t("add")}</Button>
    </div>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {targets.isError
      ? <QueryError error={targets.error} onRetry={() => void targets.refetch()} />
      : targets.isPending
        ? <p role="status">{t("loading")}</p>
        : <div className="grid gap-3 xl:grid-cols-2">
            {targets.data.map((target) => <TargetCard
              key={target.id}
              target={target}
              delivered={(host) => {
                const overview = overviews.data?.find((row) => row.hostId === host);
                return overview && !overview.unavailable
                  && overview.setVersion >= target.configVersion
                  && overview.targets.some((value) => value.id === target.id);
              }}
              edit={() => setEditing(target)}
              remove={() => setRemove(target)}
              recovery={() => setKey({ id: target.id, name: target.name })}
            />)}
          </div>}
    <h2 className="text-lg font-semibold">{t("repositories")}</h2>
    <div className="grid gap-3 xl:grid-cols-2">
      {hosts.all.map((host) => <RepositoryCard
        key={host.id}
        host={host}
        recovery={() => setKey({ id: "local", hostId: host.id, name: host.name })}
      />)}
    </div>
    {editing && <TargetDialog
      key={editing === "new" ? "new" : editing.id}
      target={editing === "new" ? undefined : editing}
      onClose={() => setEditing(undefined)}
      saved={() => {
        void targets.refetch();
        void overviews.refetch();
      }}
    />}
    {key && <RecoveryKeyDialog key={`${key.id}-${key.hostId}`} {...key} onClose={() => setKey(undefined)} />}
    <ConfirmDialog
      open={!!remove}
      onOpenChange={(open) => !open && setRemove(undefined)}
      title={t("remove")}
      description={t("retireHint")}
      confirmLabel={t("remove")}
      onConfirm={async () => {
        const feedback = captureCommandFailure(adapter);
        try {
          await adapter.deleteBackupTarget(remove!.id);
          void targets.refetch();
        } catch (cause) {
          const refs = cause instanceof BackupError
            ? [
                ...(cause.problem.hosts ?? []).map((id) => hosts.byId.get(id)?.name ?? id),
                ...(cause.problem.volumes ?? []),
              ]
            : [];
          const message = `${backupErrorMessage(cause, all)} ${refs.join(", ")}`;
          setError(message);
          feedback.error(message);
          return false;
        }
      }}
    />
  </section>;
}
function TargetCard({
  target,
  delivered,
  edit,
  remove,
  recovery,
}: {
  target: BackupTarget;
  delivered: (host: string) => boolean | undefined;
  edit: () => void;
  remove: () => void;
  recovery: () => void;
}) {
  const adapter = useAdapter(), t = useTranslations("settings.backups"), all = useTranslations(), hosts = useHostScope();
  const usable = hosts.all.filter((host) => !target.hostId || host.id === target.hostId);
  const [hostId, setHost] = useState(target.hostId ?? usable[0]?.id ?? ""),
    [activity, setActivity] = useState<{ id: string; host: string }>(),
    [error, setError] = useState<string>(),
    [busy, setBusy] = useState(false);
  return <article className="space-y-3 rounded-xl border p-4">
    <div className="flex flex-wrap items-center gap-2">
      <h3 className="break-all font-medium">{target.name}</h3>
      <ToneBadge tone="gray">{t(`kinds.${target.kind}`)}</ToneBadge>
      {target.retiredAt && <ToneBadge tone="warn">{t("retired")}</ToneBadge>}
    </div>
    <p className="break-all font-mono text-xs">
      {target.server
        ? `${target.server} · ${target.share} · ${target.path}`
        : `${hosts.byId.get(target.hostId ?? "")?.name ?? target.hostId} · ${target.path}`}
    </p>
    <ul className="space-y-2 text-sm">
      {usable.map((host) => {
        const check = target.checks?.find((value) => value.hostId === host.id);
        return <li key={host.id}>
          {host.name}: {delivered(host.id) ? t("delivered") : t("deliveryPending")}
          <p className="text-xs text-muted-foreground">
            {check
              ? `${check.ok ? t("testOk") : t("testFailed")} · ${new Date(check.at).toLocaleString()}`
              : t("untested")}
            {check && (check.earlierSettings || check.configVersion < target.configVersion)
              ? ` · ${t("earlierSettings")}`
              : ""}
          </p>
          {check?.code && <p>{all(backupCodeKey(check.code) ?? "common.commandErrors.failed")}</p>}
        </li>;
      })}
    </ul>
    <Select value={hostId} onValueChange={setHost}>
      <SelectTrigger aria-label={t("testHost")}><SelectValue /></SelectTrigger>
      <SelectContent>
        {usable.map((host) => <SelectItem key={host.id} value={host.id}>{host.name}</SelectItem>)}
      </SelectContent>
    </Select>
    <div className="flex flex-wrap gap-2">
      <Button
        variant="outline"
        disabled={busy || !!target.retiredAt || !isHostAvailable(hosts.byId.get(hostId))}
        onClick={async () => {
          setBusy(true);
          setError(undefined);
          try {
            const id = await adapter.testBackupTarget(target.id, hostId);
            setActivity({ id, host: hostId });
          } catch (cause) {
            setError(backupErrorMessage(cause, all));
          } finally {
            setBusy(false);
          }
        }}
      >
        {t("test")}
      </Button>
      <Button variant="outline" disabled={!!target.retiredAt} onClick={edit}>{t("edit")}</Button>
      <Button variant="outline" disabled={!!target.retiredAt} onClick={remove}>{t("remove")}</Button>
      <Button variant="outline" onClick={recovery}>{t("recoveryKey")}</Button>
    </div>
    {activity && <BackupOperation hostId={activity.host} activityId={activity.id} />}
    {error && <p role="alert">{error}</p>}
  </article>;
}
