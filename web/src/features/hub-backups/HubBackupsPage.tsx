// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original Hub backup page for issue #32, using free UI primitives.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Download, HardDrive, RotateCw, Upload } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useLocale, useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { hubBackupErrorCode, MAX_HUB_BACKUP_BYTES, type HubBackupPolicy, type HubBackupRun } from "@/domain/hub-backup";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Skeleton } from "@/shared/ui/skeleton";
import { Switch } from "@/shared/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";

import { useHubBackupAccess } from "./use-backup-access";

const keys = { runs: ["hub-backup-runs"], policy: ["hub-backup-policy"], storage: ["hub-backup-storage"], maintenance: ["hub-backup-maintenance"] } as const;

function formatBytes(bytes: number, locale: string): string {
  if (!Number.isFinite(bytes)) return "–";
  const unit = bytes >= 1024 ** 3 ? "GB" : bytes >= 1024 ** 2 ? "MB" : "KB";
  const divisor = unit === "GB" ? 1024 ** 3 : unit === "MB" ? 1024 ** 2 : 1024;
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(bytes / divisor)} ${unit}`;
}

function useBackupErrorText() {
  const t = useTranslations("hubBackups");
  return (error: unknown) => {
    const code = hubBackupErrorCode(error);
    if (code === "backup.storage_unavailable") return t("storageUnavailable");
    if (code === "backup.tools_unavailable") return t("toolsUnavailable");
    if (code === "backup.size_limit") return t("errors.size");
    if (code === "backup.unsupported_media_type") return t("errors.type");
    if (["backup.invalid_archive", "backup.incompatible_server", "backup.incompatible_migration"].includes(code)) return t("errors.invalid");
    if (code === "run.busy" || code === "run.cancelling") return t("errors.busy");
    return t("errors.generic");
  };
}

function PolicyEditor({ policy, disabled, onSave }: { policy: HubBackupPolicy; disabled: boolean; onSave: (policy: HubBackupPolicy) => Promise<void> }) {
  const t = useTranslations("hubBackups");
  const [enabled, setEnabled] = useState(policy.enabled);
  const [schedule, setSchedule] = useState(policy.schedule);
  const [retentionCount, setRetentionCount] = useState(String(policy.retentionCount));
  const [busy, setBusy] = useState(false);
  const [invalid, setInvalid] = useState(false);
  async function save() {
    const count = Number(retentionCount);
    if (!Number.isSafeInteger(count) || count < 1 || !schedule.trim()) { setInvalid(true); return; }
    setInvalid(false); setBusy(true);
    try { await onSave({ ...policy, enabled, schedule: schedule.trim(), retentionCount: count }); }
    finally { setBusy(false); }
  }
  return (
    <Card>
      <CardHeader><CardTitle>{t("policy")}</CardTitle><CardDescription>{t("policyHint")}</CardDescription></CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <div className="flex items-center gap-3 sm:col-span-2"><Switch id={`policy-${policy.id}`} checked={enabled} disabled={disabled || busy} onCheckedChange={setEnabled} /><Label htmlFor={`policy-${policy.id}`}>{t("enabled")}</Label></div>
        <div className="grid gap-1.5"><Label htmlFor={`schedule-${policy.id}`}>{t("schedule")}</Label><Input id={`schedule-${policy.id}`} value={schedule} onChange={(event) => setSchedule(event.target.value)} disabled={disabled || busy} /><p className="text-xs text-muted-foreground">{t("scheduleHint")}</p></div>
        <div className="grid gap-1.5"><Label htmlFor={`keep-${policy.id}`}>{t("keepCount")}</Label><Input id={`keep-${policy.id}`} type="number" min={1} step={1} value={retentionCount} onChange={(event) => setRetentionCount(event.target.value)} disabled={disabled || busy} /></div>
        {invalid && <p role="alert" className="text-sm text-destructive sm:col-span-2">{t("invalidPolicy")}</p>}
        <div className="sm:col-span-2"><Button onClick={() => void save()} disabled={disabled || busy}>{t("save")}</Button></div>
      </CardContent>
    </Card>
  );
}

function UploadDialog({ open, onOpenChange, disabled, onUploaded }: { open: boolean; onOpenChange: (open: boolean) => void; disabled: boolean; onUploaded: () => void }) {
  const t = useTranslations("hubBackups");
  const adapter = useAdapter();
  const errorText = useBackupErrorText();
  const [file, setFile] = useState<File | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function upload() {
    if (!file) return;
    if (file.size > MAX_HUB_BACKUP_BYTES) { setError(t("errors.size")); return; }
    if (file.type && file.type !== "application/octet-stream") { setError(t("errors.type")); return; }
    setError(""); setProgress(0); setBusy(true);
    try {
      await adapter.uploadHubBackup(file, setProgress);
      toast.success(t("uploadSuccess"));
      onUploaded(); onOpenChange(false); setFile(null);
    } catch (cause) { setError(errorText(cause)); }
    finally { setBusy(false); }
  }
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!busy) onOpenChange(next); }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{t("uploadTitle")}</DialogTitle><DialogDescription>{t("uploadHint")}</DialogDescription></DialogHeader>
        <div className="grid gap-2"><Label htmlFor="hub-backup-file">{t("chooseFile")}</Label><Input id="hub-backup-file" type="file" accept=".dump,application/octet-stream" disabled={busy} onChange={(event) => { setFile(event.target.files?.[0] ?? null); setError(""); }} /></div>
        {progress !== null && <div role="status" className="text-sm">{t("uploadProgress", { percent: progress })}<progress className="mt-2 w-full" max={100} value={progress} /></div>}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>{t("cancel")}</Button><Button disabled={disabled || !file || busy} onClick={() => void upload()}>{t("uploadAction")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RestoreDialog({ run, onClose, onStarted }: { run: HubBackupRun | null; onClose: () => void; onStarted: (activityId: string) => void }) {
  const t = useTranslations("hubBackups");
  const locale = useLocale();
  const adapter = useAdapter();
  const errorText = useBackupErrorText();
  const [accepted, setAccepted] = useState(false);
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function restore() {
    if (!run || !accepted || typed !== run.id) return;
    setBusy(true); setError("");
    try { const operation = await adapter.restoreHubBackup(run.id); onStarted(operation.activityId); onClose(); }
    catch (cause) { setError(errorText(cause)); }
    finally { setBusy(false); }
  }
  return (
    <Dialog open={!!run} onOpenChange={(open) => { if (!open && !busy) onClose(); }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{t("restoreTitle")}</DialogTitle><DialogDescription>{run ? t("restoreConsequences", { name: run.id, created: new Date(run.createdAt).toLocaleString(locale) }) : ""}</DialogDescription></DialogHeader>
        <div className="flex items-start gap-2"><Checkbox id="restore-safety" checked={accepted} onCheckedChange={(value) => setAccepted(value === true)} /><Label htmlFor="restore-safety" className="leading-snug">{t("restoreSafety")}</Label></div>
        <div className="grid gap-2"><Label htmlFor="restore-id">{t("restoreType")}</Label><Input id="restore-id" autoComplete="off" value={typed} onChange={(event) => setTyped(event.target.value)} placeholder={run?.id} /></div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button variant="outline" disabled={busy} onClick={onClose}>{t("cancel")}</Button><Button variant="destructive" disabled={!run || !accepted || typed !== run.id || busy} onClick={() => void restore()}>{t("restoreAction")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function HubBackupsPage() {
  const t = useTranslations("hubBackups");
  const locale = useLocale();
  const adapter = useAdapter();
  const client = useQueryClient();
  const navigate = useNavigate();
  const errorText = useBackupErrorText();
  const access = useHubBackupAccess();
  const [uploadOpen, setUploadOpen] = useState(false);
  const [restoreRun, setRestoreRun] = useState<HubBackupRun | null>(null);
  const [creating, setCreating] = useState(false);
  const [restoreStarted, setRestoreStarted] = useState(false);
  const [restoreActivityId, setRestoreActivityId] = useState("");
  const seenMaintenance = useRef(false);
  const [actionError, setActionError] = useState("");
  const maintenance = useQuery({ queryKey: keys.maintenance, queryFn: () => adapter.getHubBackupMaintenance(), refetchInterval: 1000, retry: false });
  const canRead = access.data?.read === true;
  const runs = useQuery({ queryKey: keys.runs, queryFn: () => adapter.listHubBackups(), enabled: canRead && !maintenance.data?.active, refetchInterval: 2500 });
  const policies = useQuery({ queryKey: keys.policy, queryFn: () => adapter.getHubBackupPolicies(), enabled: canRead && !maintenance.data?.active });
  const storage = useQuery({ queryKey: keys.storage, queryFn: () => adapter.getHubBackupStorage(), enabled: canRead && !maintenance.data?.active, refetchInterval: 5000 });
  const activityIds = (runs.data ?? []).filter((run) => run.status !== "succeeded").map((run) => run.activityId).filter((id): id is string => !!id);
  const activity = useQuery({ queryKey: ["hub-backup-activity", activityIds.join(",")], enabled: canRead && activityIds.length > 0 && !maintenance.data?.active,
    queryFn: async () => Object.fromEntries(await Promise.all(activityIds.map(async (id) => [id, await adapter.getHubBackupActivityStatus(id).catch(() => "")] as const))),
    refetchInterval: (runs.data ?? []).some((run) => run.status === "running") ? 2000 : false,
  });
  const restoreActivity = useQuery({ queryKey: ["hub-restore-activity", restoreActivityId], enabled: !!restoreActivityId && !maintenance.data?.active,
    queryFn: () => adapter.getHubBackupActivityStatus(restoreActivityId), refetchInterval: (query) => ["success", "succeeded", "failed", "cancelled", "canceled"].includes(query.state.data ?? "") ? false : 1000, retry: false,
  });
  const restoreFailed = ["failed", "cancelled", "canceled"].includes(restoreActivity.data ?? "");

  useEffect(() => {
    if (maintenance.data?.active) seenMaintenance.current = true;
    else if (seenMaintenance.current && maintenance.data) navigate("/login", { replace: true });
  }, [maintenance.data, navigate]);
  useEffect(() => {
    if (!restoreStarted || seenMaintenance.current || maintenance.data?.active) return;
    if (restoreActivity.data === "success" || restoreActivity.data === "succeeded") navigate("/login", { replace: true });
    if (restoreActivity.error instanceof AdapterOutcomeError && restoreActivity.error.outcome.kind === "error" && restoreActivity.error.outcome.status === 401) navigate("/login", { replace: true });
  }, [restoreStarted, maintenance.data?.active, restoreActivity.data, restoreActivity.error, navigate]);

  const refresh = () => { void client.invalidateQueries({ queryKey: keys.runs }); void client.invalidateQueries({ queryKey: keys.storage }); };
  const unavailable = storage.data?.reasonCode === "backup.storage_unavailable" ? t("storageUnavailable") : storage.data?.reasonCode === "backup.tools_unavailable" ? t("toolsUnavailable") : "";
  const canManage = access.data?.manage && storage.data?.available && (!restoreStarted || restoreFailed);
  const canRestore = access.data?.restore && storage.data?.available && (!restoreStarted || restoreFailed);

  async function create() {
    setActionError(""); setCreating(true);
    try { await adapter.createHubBackup(); refresh(); }
    catch (cause) { setActionError(errorText(cause)); }
    finally { setCreating(false); }
  }
  async function savePolicy(policy: HubBackupPolicy) {
    setActionError("");
    try {
      await adapter.saveHubBackupPolicies(policies.data?.length ? policies.data.map((entry) => entry.id === policy.id ? policy : entry) : [policy]);
      await client.invalidateQueries({ queryKey: keys.policy });
      toast.success(t("saved"));
    } catch (cause) { setActionError(errorText(cause)); }
  }
  async function download(id: string) {
    setActionError("");
    try { await adapter.downloadHubBackup(id); } catch (cause) { setActionError(errorText(cause)); }
  }

  if (maintenance.data?.active || (restoreStarted && !restoreFailed)) {
    const phase = maintenance.data?.phase ?? "draining";
    return <section role="status" className="mx-auto max-w-2xl space-y-4 rounded-xl border bg-card p-6">
      <h1 className="text-2xl font-semibold">{t("maintenanceTitle")}</h1>
      <p className="text-sm text-muted-foreground">{t("maintenanceHint")}</p>
      <p className="font-medium">{t(`maintenancePhase.${phase}`)}</p>
      <progress className="w-full" max={3} value={phase === "draining" ? 1 : phase === "applying" ? 2 : 3} />
      {phase === "blocked" && <p className="text-sm text-destructive">{t("maintenancePhase.blocked")}</p>}
    </section>;
  }
  if (access.isPending || maintenance.isPending) return <Skeleton className="h-80 w-full" />;
  if (access.isError || maintenance.isError) return <QueryError onRetry={() => { void access.refetch(); void maintenance.refetch(); }} />;
  if (!canRead) return <p role="alert" className="rounded-lg border p-6">{t("accessDenied")}</p>;
  return <>
    <PageHeader title={t("title")} description={t("description")} actions={<><Button variant="outline" disabled={!canRestore} onClick={() => setUploadOpen(true)}><Upload data-icon="inline-start" />{t("upload")}</Button><Button disabled={!canManage || creating} onClick={() => void create()}><Archive data-icon="inline-start" />{creating ? t("creating") : t("create")}</Button></>} />
    <div className="mb-5 grid gap-3 rounded-lg border bg-card/50 p-4 text-sm text-muted-foreground"><p>{t("scope")}</p><p>{t("sensitive")}</p></div>
    {unavailable && <p role="alert" className="mb-5 rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">{unavailable}</p>}
    {(actionError || restoreFailed) && <p role="alert" className="mb-5 rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">{actionError || t("errors.generic")}</p>}
    <Card className="mb-5"><CardHeader><CardTitle className="flex items-center gap-2"><HardDrive className="size-4" />{t("storage")}</CardTitle><CardDescription>{storage.data ? t("storageUsage", { used: formatBytes(storage.data.usedBytes, locale), free: formatBytes(storage.data.freeBytes, locale) }) : "–"}</CardDescription></CardHeader></Card>
    <Card className="mb-5"><CardHeader className="flex flex-row items-center justify-between"><CardTitle>{t("archives")}</CardTitle><Button size="sm" variant="ghost" onClick={refresh} aria-label={t("retry")}><RotateCw /></Button></CardHeader><CardContent>
      {runs.isError ? <QueryError onRetry={() => void runs.refetch()} /> : runs.isPending ? <Skeleton className="h-48 w-full" /> : !runs.data?.length ? <EmptyState icon={<Archive />} title={t("emptyTitle")} description={t("emptyHint")} /> :
        <div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>{t("name")}</TableHead><TableHead>{t("created")}</TableHead><TableHead>{t("size")}</TableHead><TableHead>{t("trigger")}</TableHead><TableHead>{t("status")}</TableHead><TableHead>{t("actions")}</TableHead></TableRow></TableHeader><TableBody>{runs.data.map((run) => {
          const state = run.activityId ? activity.data?.[run.activityId] : "";
          const status = state === "queued" ? "queued" : state === "cancelled" || state === "canceled" ? "cancelled" : run.status;
          return <TableRow key={run.id}><TableCell className="font-mono text-xs">{run.id}</TableCell><TableCell>{new Date(run.createdAt).toLocaleString(locale)}</TableCell><TableCell>{run.size ? formatBytes(run.size, locale) : "–"}</TableCell><TableCell>{t(`triggerValues.${run.trigger}`)}</TableCell><TableCell><ToneBadge tone={status === "succeeded" ? "ok" : status === "failed" ? "err" : "gray"}>{t(`statusValues.${status}`)}</ToneBadge></TableCell><TableCell><div className="flex gap-2"><Button size="sm" variant="outline" disabled={status !== "succeeded"} onClick={() => void download(run.id)}><Download data-icon="inline-start" />{t("download")}</Button><Button size="sm" variant="outline" disabled={status !== "succeeded" || !canRestore} onClick={() => setRestoreRun(run)}>{t("restore")}</Button></div></TableCell></TableRow>;
        })}</TableBody></Table></div>}
    </CardContent></Card>
    {policies.isError ? <QueryError onRetry={() => void policies.refetch()} /> : policies.isPending ? <Skeleton className="h-52 w-full" /> : (policies.data?.length ? policies.data : [{ id: "", enabled: false, schedule: "0 0 3 * * *", retentionCount: 7 }]).map((policy) => <PolicyEditor key={`${policy.id}-${policy.schedule}-${policy.retentionCount}-${policy.enabled}`} policy={policy} disabled={!canManage} onSave={savePolicy} />)}
    <UploadDialog open={uploadOpen} onOpenChange={setUploadOpen} disabled={!canRestore} onUploaded={refresh} />
    <RestoreDialog key={restoreRun?.id ?? "none"} run={restoreRun} onClose={() => setRestoreRun(null)} onStarted={(activityId) => { setRestoreActivityId(activityId); setRestoreStarted(true); void maintenance.refetch(); }} />
  </>;
}
