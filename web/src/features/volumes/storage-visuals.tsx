import { useCapability } from "@/data/use-capability";
import { Archive, Box, Database, File, FolderInput, Layers, Loader2 } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatRelative } from "@/domain/format";
import type { Volume } from "@/domain/volumes";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";

/** Storage kind chip: volume, directory bind mount or single-file bind mount. */
export function KindBadge({ volume, className }: { volume: Pick<Volume, "kind" | "fileBind">; className?: string }) {
  const t = useTranslations("volumes.kind");
  if (volume.kind === "volume")
    return (
      <ToneBadge tone="accent" className={className}>
        <Database aria-hidden />
        {t("volume")}
      </ToneBadge>
    );
  return (
    <ToneBadge tone="teal" className={className}>
      {volume.fileBind ? <File aria-hidden /> : <FolderInput aria-hidden />}
      {volume.fileBind ? t("file") : t("bind")}
    </ToneBadge>
  );
}

export function BackupBadge({ volume }: { volume: Pick<Volume, "backup" | "lastBackup" | "backupUnavailable" | "kind" | "hostId"> }) {
  const t = useTranslations("volumes.backupState");
  const backup = useCapability("getBackupOverview", { kind: "host", id: volume.hostId });
  if (volume.kind === "bind") return <span className="text-xs text-muted-foreground">{t("namedVolumesOnly")}</span>;
  if (volume.backupUnavailable || !backup.available) return <span className="text-xs text-muted-foreground" title={backup.reason}>{t("unavailable")}</span>;
  if (volume.lastBackup?.status === "partial") return <ToneBadge tone="warn">{t("partial")}</ToneBadge>;
  if (volume.lastBackup?.status === "unavailable") return <ToneBadge tone="gray">{t("unavailable")}</ToneBadge>;
  if (!volume.backup && !volume.lastBackup) return <span className="text-xs text-muted-foreground">{t("none")}</span>;
  if (volume.backup && !volume.backup.enabled) return <ToneBadge tone="gray">{t("disabled")}</ToneBadge>;
  const last = volume.lastBackup;
  if (last?.status === "running")
    return (
      <ToneBadge tone="accent">
        <Loader2 className="animate-spin" aria-hidden />
        {t("running")}
      </ToneBadge>
    );
  if (last?.status === "failed")
    return (
      <ToneBadge tone="err">
        <Archive aria-hidden />
        {t("failed")}
      </ToneBadge>
    );
  return (
    <ToneBadge tone="ok">
      <Archive aria-hidden />
      {last ? t("ok", { time: formatRelative(last.finishedAt ?? last.startedAt) }) : t("ok", { time: "–" })}
    </ToneBadge>
  );
}

/** Consumer links: stack services open their stack page, others the container inspector. */
export function ConsumerChips({ volume, max = 3, className }: { volume: Pick<Volume, "consumers">; max?: number; className?: string }) {
  const t = useTranslations("volumes");
  if (!volume.consumers.length) return <span className="text-xs text-muted-foreground">{t("unused")}</span>;
  const shown = volume.consumers.slice(0, max);
  const rest = volume.consumers.length - shown.length;
  return (
    <span className={cn("flex flex-wrap items-center gap-1", className)}>
      {shown.map((c) => (
        <Link
          key={`${c.containerId}-${c.target}`}
          to={c.stackId ? `/stacks/${c.stackId}` : `/containers?c=${c.containerId}`}
          data-row-stop
          title={`${c.containerName} → ${c.target}${c.readOnly ? ` (${t("readOnly")})` : ""}`}
          className="inline-flex max-w-full items-center gap-1 rounded-md border bg-background/50 px-1.5 py-0.5 text-[11.5px] outline-none hover:border-primary/50 hover:text-primary-text focus-visible:ring-2 focus-visible:ring-ring"
        >
          {c.stackId ? <Layers className="size-3 shrink-0 text-muted-foreground" aria-hidden /> : <Box className="size-3 shrink-0 text-muted-foreground" aria-hidden />}
          <span className="min-w-0 [overflow-wrap:anywhere]">{c.stackId && c.service ? `${c.stackName ?? c.stackId}/${c.service}` : c.containerName}</span>
          {!c.running && <span className="size-1.5 shrink-0 rounded-full bg-muted-foreground" aria-hidden />}
        </Link>
      ))}
      {rest > 0 && <span className="text-[11px] text-muted-foreground">{t("more", { count: rest })}</span>}
    </span>
  );
}
