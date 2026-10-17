import { Database, FolderInput, FolderOpen } from "lucide-react";
import { memo, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatBytes } from "@/domain/format";
import type { Host } from "@/domain/model";
import { volumeLabel, type Volume } from "@/domain/volumes";
import { ActionsMenu } from "@/shared/components/action-menu";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewRowView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import { IconTile } from "@/shared/components/tone";
import type { FitView } from "@/shared/lib/fit-view";
import { useRowNavigation } from "@/shared/lib/row-navigation";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { filesHref } from "./files/files-href";
import { BackupBadge, ConsumerChips, KindBadge } from "./storage-visuals";
import { useVolumeMenu } from "./volume-menu";

interface VolumeRowProps {
  volume: Volume;
  host: Host | undefined;
  view: FitView;
  columns: OverviewColumn[];
  inBlock?: boolean;
  onRemove: (v: Volume) => void;
  onBackup: (v: Volume) => void;
}

/** One volume or bind mount as table row or card; the entry opens the volume page. */
export const VolumeRow = memo(function VolumeRow({ volume: v, host, view, columns, inBlock, onRemove, onBackup }: VolumeRowProps) {
  const t = useTranslations("volumes");
  const menu = useVolumeMenu(v, host, { remove: onRemove, backup: onBackup });
  const rowNav = useRowNavigation();
  const href = `/volumes/${v.id}`;
  const condensed = view === "condensed";
  const kind = <KindBadge volume={v} />;
  const cells: Record<string, ReactNode> = {
    name: (
      <div className="flex items-center gap-3">
        {!inBlock && view !== "cards" && (
          <IconTile color={host?.color} size="md">
            {v.kind === "volume" ? <Database /> : <FolderInput />}
          </IconTile>
        )}
        <div className="min-w-0">
          <Link to={href} aria-label={t("rowOpen", { name: volumeLabel(v) })} className="block rounded-sm font-semibold tracking-tight [overflow-wrap:anywhere] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring">
            {volumeLabel(v)}
          </Link>
          <span className="block font-mono text-[11.5px] text-muted-foreground [overflow-wrap:anywhere]">{v.kind === "volume" ? v.name : v.source}</span>
          {condensed && (
            <span className="mt-1 flex flex-wrap items-center gap-2">
              {kind}
              {!inBlock && <HostChip host={host} />}
            </span>
          )}
        </div>
      </div>
    ),
    kind,
    host: <HostChip host={host} />,
    consumers: <ConsumerChips volume={v} />,
    size: <span className="font-mono text-xs tabular">{v.sizeUnavailable ? t("unknown") : formatBytes(v.sizeBytes)}</span>,
    backup: <BackupBadge volume={v} />,
  };
  return (
    <OverviewRowView
      view={view}
      columns={columns}
      cells={cells}
      pad={inBlock ? "py-2" : "py-3"}
      hostColor={host?.color}
      onClick={rowNav(href).onClick}
      menu={menu}
      menuTitle={volumeLabel(v)}
      actions={
        <span className="inline-flex items-center">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button asChild variant="ghost" size="icon" className="size-8">
                <Link to={filesHref(v)} aria-label={`${t("openFiles")}: ${volumeLabel(v)}`}>
                  <FolderOpen />
                </Link>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("openFiles")}</TooltipContent>
          </Tooltip>
          <ActionsMenu groups={menu} label={t("menu.label", { name: volumeLabel(v) })} />
        </span>
      }
    />
  );
});
