import { useQuery } from "@tanstack/react-query";
import { Eraser, ShieldCheck, Unplug } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { formatBytes } from "@/domain/format";
import type { Host } from "@/domain/model";
import { volumeLabel } from "@/domain/volumes";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
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
 * why, and which hosts are skipped. The plan comes from the adapter and
 * refreshes with every volume change; the prune checks each volume again.
 */
export function PruneVolumesDialog({ open, onOpenChange, hosts }: Props) {
  const t = useTranslations("volumes.prune");
  const adapter = useAdapter();
  const cmd = useVolumeCommands();
  const hostIds = hosts.map((h) => h.id);
  const plan = useQuery({ queryKey: [...queryKeys.volumes, "prune-plan", ...hostIds], queryFn: () => adapter.planVolumePrune({ hostIds }), enabled: open, staleTime: 0 });
  const [excluded, setExcluded] = useState<ReadonlySet<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const hostName = (id: string) => hosts.find((h) => h.id === id)?.name ?? id;

  const candidates = plan.data?.candidates ?? [];
  const chosen = candidates.filter((v) => !excluded.has(v.id));
  const size = chosen.reduce((n, v) => n + v.sizeBytes, 0);
  const inUse = plan.data?.kept.filter((k) => k.reason === "in-use") ?? [];
  const binds = plan.data?.kept.filter((k) => k.reason === "bind").length ?? 0;
  const skipped = plan.data?.skippedHosts ?? [];

  const close = (o: boolean) => {
    onOpenChange(o);
    if (!o) setExcluded(new Set());
  };

  const confirm = async () => {
    setBusy(true);
    const result = await cmd.prune(hostIds, chosen.map((v) => v.id));
    setBusy(false);
    if (!result) return;
    if (result.removed.length) toast.success(t("done", { count: result.removed.length, size: formatBytes(result.removed.reduce((n, r) => n + r.sizeBytes, 0)) }));
    if (result.skipped.length) toast.warning(t("changed", { count: result.skipped.length }));
    close(false);
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-xl" data-surface="volume-prune-dialog">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        {plan.isPending ? (
          <div aria-busy="true" className="flex flex-col gap-2">
            <Skeleton className="h-6 w-2/3" />
            <Skeleton className="h-24" />
          </div>
        ) : plan.isError ? (
          <QueryError onRetry={() => void plan.refetch()} />
        ) : (
          <div className="flex flex-col gap-3 text-sm">
            <p className="text-xs text-muted-foreground">{t("scope", { hosts: hosts.filter((h) => !skipped.includes(h.id)).map((h) => h.name).join(", ") || "–" })}</p>
            {skipped.length > 0 && (
              <p className="flex gap-1.5 text-xs text-warning">
                <Unplug className="mt-0.5 size-3.5 shrink-0" aria-hidden />
                {t("skippedHosts", { hosts: skipped.map(hostName).join(", ") })}
              </p>
            )}
            <section className="rounded-xl border bg-muted/15 p-3" aria-label={t("candidates")}>
              <p className="font-medium">{candidates.length ? t("summary", { count: chosen.length, total: candidates.length, size: formatBytes(size) }) : t("nothing")}</p>
              {candidates.length > 0 && (
                <ul className="mt-2 flex max-h-56 flex-col overflow-auto">
                  {candidates.map((v) => {
                    const id = `prune-${v.id}`;
                    return (
                      <li key={v.id} className="flex items-center gap-2.5 border-t border-border/50 py-1.5 first:border-0">
                        <Checkbox
                          id={id}
                          checked={!excluded.has(v.id)}
                          onCheckedChange={(on) =>
                            setExcluded((s) => {
                              const next = new Set(s);
                              if (on) next.delete(v.id);
                              else next.add(v.id);
                              return next;
                            })
                          }
                        />
                        <label htmlFor={id} className="min-w-0 flex-1 cursor-pointer">
                          <span className="block font-mono text-xs [overflow-wrap:anywhere]">{volumeLabel(v)}</span>
                          <span className="text-[11px] text-muted-foreground">{hostName(v.hostId)}</span>
                        </label>
                        <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">{formatBytes(v.sizeBytes)}</span>
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
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("cancel")}
          </Button>
          <Button type="button" variant="destructive" disabled={!chosen.length || busy || plan.isPending} onClick={() => void confirm()}>
            {busy ? <Spinner data-icon="inline-start" /> : <Eraser data-icon="inline-start" />}
            {t("confirm", { count: chosen.length })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
