// Source: docklet-next/docs/design/backup-completion.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original /backups overview figures, unprotected entries and target summaries (#329).

import type { BackupTarget } from "@/domain/settings";
import type { BackupOverview, Volume } from "@/domain/volumes";

/** Entries a plan could protect: existing volumes and binds the agent accepted as directory or regular file. */
function protectable(v: Volume): boolean {
  return v.kind === "volume" ? v.volumeExists !== false : !!v.bind?.eligible && (v.bind.kind === "directory" || v.bind.kind === "file");
}

/**
 * Overview figures over hosts whose backup data is available; other hosts enter no figure.
 * Plans include manual ones; partial runs count as failed; unprotected entries have consumers and no plan.
 */
export function backupFigures(overviews: readonly BackupOverview[], inventory: readonly Volume[]) {
  const available = overviews.filter((o) => !o.unavailable);
  const hosts = new Set(available.map((o) => o.hostId));
  const planned = available.flatMap((o) => o.volumes).filter((v) => v.backup);
  const plans = new Set(planned.map((v) => v.id));
  const status = (v: Volume) => v.lastBackup?.status;
  return {
    plans: planned.length,
    succeeded: planned.filter((v) => status(v) === "succeeded").length,
    failed: planned.filter((v) => status(v) === "failed" || status(v) === "partial").length,
    unprotected: inventory.filter((v) => hosts.has(v.hostId) && v.consumers.length > 0 && !plans.has(v.id) && !v.backup && protectable(v)),
  };
}

export function newestCheck(target: Pick<BackupTarget, "checks">) {
  return (target.checks ?? []).reduce<NonNullable<BackupTarget["checks"]>[number] | undefined>((best, check) => (!best || check.at > best.at ? check : best), undefined);
}

/** Display address of a target: share or export for remote targets, owner host and path otherwise. */
export function targetAddress(target: Pick<BackupTarget, "kind" | "server" | "share" | "path" | "hostId">, hostName?: string): string {
  const sub = target.path ? `/${target.path.replace(/^\/+/, "")}` : "";
  if (target.kind === "smb") return `//${target.server ?? ""}/${target.share ?? ""}${sub}`;
  if (target.kind === "nfs") return `${target.server ?? ""}:${(target.share ?? "").replace(/\/+$/, "")}${sub}`;
  return `${hostName ?? target.hostId ?? ""} · ${target.path}`;
}
