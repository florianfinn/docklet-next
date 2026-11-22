import { PRUNE_CANDIDATE_KEYS, PRUNE_SKIP_KEYS, PRUNE_FAILURE_KEYS } from "@/data/wire-catalogue-states";
import { scopedKey, wireKey } from "@/data/wire-catalogue";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { useQuery } from "@tanstack/react-query";
import { Eraser, ShieldCheck, Unplug } from "lucide-react";
import { useId, useState } from "react";
import { isHostAvailable } from "@/domain/hosts";
import { useTranslations } from "use-intl";

import type { VolumePruneResult } from "@/data/api";
import { useCapability } from "@/data/use-capability";
import { useAdapter } from "@/data/use-adapter";
import { formatBytes } from "@/domain/format";
import type { Host } from "@/domain/model";
import { volumeLabel } from "@/domain/volumes";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Skeleton } from "@/shared/ui/skeleton";
import { Spinner } from "@/shared/ui/spinner";

import { useVolumeCommands } from "./use-volume-commands";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Hosts of the global filter; the adapter skips disconnected ones. */
  hosts: Host[];
}

/**
 * Prune preview: candidates with size and a choice per volume, what stays and
 * why, and which hosts are skipped. The snapshot stays stable while the user
 * confirms; execution rechecks identity and usage and reports every deviation.
 */
const identity = (v: { hostId: string; name: string; createdAt?: string }) => JSON.stringify([v.hostId, v.name, v.createdAt ?? ""]);
// Host order, names and status never reset the dialog; a different set of hosts does.
const scopeKey = (hosts: Host[]) => hosts.map((h) => h.id).sort().join("|");

function PruneVolumesForm({ open, onOpenChange, hosts }: Props) {
  const t = useTranslations("volumes.prune");
  const adapter = useAdapter();
  const cmd = useVolumeCommands();
  const hostIds = hosts.map((h) => h.id);
  // A host coming online or going offline reads a new plan without resetting earlier choices.
  const available = hosts.filter(isHostAvailable).map((h) => h.id).sort();
  const session = useId();
  const feedback = useDialogCommand(open, session);
  const [all, setAll] = useState(false);
  const capability = useCapability("pruneVolumes");
  const plan = useQuery({ queryKey: ["volume-prune-plan", session, all, scopeKey(hosts), available.join(",")], queryFn: () => adapter.planVolumePrune({ hostIds, all }), staleTime: Infinity, gcTime: 0 });
  const [result, setResult] = useState<VolumePruneResult>();
  // Choices per candidate identity (host, name, creation). The first plan selects its candidates; candidates that
  // appear later start unselected, so a refresh never deletes something nobody chose. Only arrivals from a host
  // change are marked new; the named-volume toggle adds what the user just asked to see.
  const [choices, setChoices] = useState<ReadonlyMap<string, boolean>>(new Map());
  const [fresh, setFresh] = useState<ReadonlySet<string>>(new Set());
  const [adopted, setAdopted] = useState<{ data: unknown; all: boolean }>();
  if (plan.data && plan.data !== adopted?.data) {
    const first = adopted === undefined, mark = !first && adopted.all === all, next = new Map(choices), added = new Set(fresh);
    for (const v of plan.data.candidates) if (!next.has(identity(v))) { next.set(identity(v), first); if (mark) added.add(identity(v)); }
    setAdopted({ data: plan.data, all }); setChoices(next); setFresh(added);
  }
  const [busy, setBusy] = useState(false);
  const hostName = (id: string) => hosts.find((h) => h.id === id)?.name ?? id;

  const candidates = plan.data?.candidates ?? [];
  const chosen = candidates.filter((v) => choices.get(identity(v)) === true);
  const size = chosen.some((v) => v.sizeUnavailable) ? t("sizeUnknown") : formatBytes(chosen.reduce((n, v) => n + v.sizeBytes, 0));
  const inUse = plan.data?.kept.filter((k) => k.reason === "in-use") ?? [];
  const binds = plan.data?.kept.filter((k) => k.reason === "bind").length ?? 0;
  const skipped = plan.data?.skippedHosts ?? [];

  const close = (o: boolean) => {
    feedback.change(o, onOpenChange);
  };

  const confirm = async () => {
    if (busy || plan.isFetching || !capability.available) return;
    setBusy(true);
    const result = await feedback.run(() => cmd.prune({ hostIds, volumeIds: chosen.map((v) => v.id), candidates: chosen.map(({ id, hostId, name, createdAt }) => ({ id, hostId, name, createdAt })), all: plan.data?.all ?? all }));
    setBusy(false);
    if (!result) return;
    setResult(result);
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl" data-surface="volume-prune-dialog">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        {result ? (
          <div className="flex max-h-[65vh] flex-col gap-3 overflow-auto text-sm" role="status">
            <p>{t("reclaimedLowerBound", { size: formatBytes(result.spaceReclaimed ?? 0) })}</p>
            <section aria-label={t("removed")}><h3 className="font-medium">{t("removed")} ({result.removed.length})</h3><ul>{result.removed.map((v) => <li key={v.id}>{v.name} · {hostName(v.hostId)}</li>)}</ul></section>
            <section aria-label={t("skipped")}><h3 className="font-medium">{t("skipped")} ({result.skipped.length})</h3><ul>{result.skipped.map((v) => <li key={v.id}>{candidates.find((c) => c.id === v.id)?.name ?? v.id} · {hostName(candidates.find((c) => c.id === v.id)?.hostId ?? v.id.split("~")[0]!)} · {t(scopedKey(PRUNE_SKIP_KEYS[v.reason as keyof typeof PRUNE_SKIP_KEYS] ?? `volumes.prune.skipReasons.${v.reason}`, "volumes.prune."))}</li>)}</ul></section>
            <section aria-label={t("failed")}><h3 className="font-medium">{t("failed")} ({result.failed?.length ?? 0})</h3><ul>{result.failed?.map((v) => <li key={v.id}>{v.name} · {hostName(v.hostId)} · {wireKey(PRUNE_FAILURE_KEYS, v.code) ? t(scopedKey(PRUNE_FAILURE_KEYS[v.code as keyof typeof PRUNE_FAILURE_KEYS], "volumes.prune.")) : t("operationFailed")}</li>)}</ul></section>
          </div>
        ) : plan.isPending ? (
          <div aria-busy="true" className="flex flex-col gap-2">
            <Skeleton className="h-6 w-2/3" />
            <Skeleton className="h-24" />
          </div>
        ) : plan.isError ? (
          <QueryError error={plan.error} onRetry={() => void plan.refetch()} />
        ) : (
          <div className="flex flex-col gap-3 text-sm">
            <label className="flex items-center gap-2"><Checkbox checked={all} disabled={busy} onCheckedChange={(checked) => setAll(checked === true)} />{t("includeNamed")}</label>
            <p className="text-xs text-muted-foreground">{t("scope", { hosts: hosts.filter((h) => !skipped.includes(h.id) && !plan.data?.failedHosts?.some((f) => f.hostId === h.id)).map((h) => h.name).join(", ") || "–" })}</p>
            {plan.data?.failedHosts?.map((failure) => <p role="alert" key={failure.hostId}>{t("hostFailed", { host: hostName(failure.hostId) })}</p>)}
            {skipped.length > 0 && (
              <p className="flex gap-1.5 text-xs text-warning">
                <Unplug className="mt-0.5 size-3.5 shrink-0" aria-hidden />
                {t("skippedHosts", { hosts: skipped.map(hostName).join(", ") })}
              </p>
            )}
            <section className="rounded-xl border bg-muted/15 p-3" aria-label={t("candidates")}>
              <p className="font-medium">{candidates.length ? t("summary", { count: chosen.length, total: candidates.length, size }) : t("nothing")}</p>
              {candidates.length > 0 && (
                <ul className="mt-2 flex max-h-56 flex-col overflow-auto">
                  {candidates.map((v) => {
                    const id = `prune-${v.id}`;
                    const reason = plan.data?.candidateReasons?.[v.id];
                    return (
                      <li key={v.id} className="flex items-center gap-2.5 border-t border-border/50 py-1.5 first:border-0">
                        <Checkbox
                          id={id}
                          disabled={busy}
                          checked={choices.get(identity(v)) === true}
                          onCheckedChange={(on) => setChoices((m) => new Map(m).set(identity(v), on === true))}
                        />
                        <label htmlFor={id} className="min-w-0 flex-1 cursor-pointer">
                          <span className="block font-mono text-xs [overflow-wrap:anywhere]">{volumeLabel(v)}</span>
                          <span className="text-[11px] text-muted-foreground">{fresh.has(identity(v)) && <span className="font-medium text-info">{t("new")} · </span>}{hostName(v.hostId)}{reason ? ` · ${t(scopedKey(PRUNE_CANDIDATE_KEYS[reason], "volumes.prune."))}` : ""}</span>
                        </label>
                        <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">{v.sizeUnavailable ? t("sizeUnknown") : formatBytes(v.sizeBytes)}</span>
                      </li>
                    );
                  })}
                </ul>
              )}
            </section>
            {(inUse.length > 0 || binds > 0) && (
              <section className="flex gap-2 text-xs text-success" aria-label={t("kept")}>
                <ShieldCheck className="mt-0.5 size-4 shrink-0" aria-hidden />
                <div className="flex flex-col gap-1">
                  {inUse.length > 0 && <span>{t("keepInUse", { count: inUse.length, stopped: inUse.filter((k) => k.volume.consumers.every((c) => !c.running)).length })}</span>}
                  {binds > 0 && <span>{t("keepBinds", { count: binds })}</span>}
                </div>
              </section>
            )}
          </div>
        )}
        <DialogCommandFooter feedback={feedback}>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {result ? t("close") : t("cancel")}
          </Button>
          {!result && <Button type="button" variant="destructive" disabled={!chosen.length || busy || plan.isFetching || plan.isError || !capability.available} title={capability.reason} onClick={() => void confirm()}>
            {busy ? <Spinner data-icon="inline-start" /> : <Eraser data-icon="inline-start" />}
            {t("confirm", { count: chosen.length })}
          </Button>}
        </DialogCommandFooter>
      </DialogContent>
    </Dialog>
  );
}

export function PruneVolumesDialog(props: Props) {
  return props.open ? <PruneVolumesForm key={scopeKey(props.hosts)} {...props} /> : null;
}
