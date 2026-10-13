import { Box, Layers } from "lucide-react";
import { memo, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { formatPorts, formatRelative } from "@/domain/format";
import type { ContainerInfo, Host } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE } from "@/domain/status";
import { versionLabel } from "@/domain/updates";
import { ActionsMenu } from "@/shared/components/action-menu";
import { ContainerLoad } from "@/shared/components/container-load";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewRowView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import { IconTile, ToneBadge, ToneDot } from "@/shared/components/tone";
import { UpdateIndicator } from "@/shared/components/updates/update-indicator";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import type { FitView } from "@/shared/lib/fit-view";
import { isNestedInteractive } from "@/shared/lib/row-navigation";
import { cn } from "@/shared/lib/utils";
import type { OverviewMode } from "@/shared/preferences/preferences-store";
import { Checkbox } from "@/shared/ui/checkbox";

import { useContainerMenu } from "./container-menu";
import { OriginBadge } from "./OriginBadge";

interface ContainerRowProps {
  container: ContainerInfo;
  host: Host | undefined;
  mode: OverviewMode;
  view: FitView;
  columns: OverviewColumn[];
  selecting: boolean;
  selected: boolean;
  onSelect: (id: string, on: boolean) => void;
  onOpen: (id: string) => void;
  onEdit: (id: string) => void;
  /** Row inside a host block: the host is the block heading. */
  inBlock?: boolean;
}

/** One container as table row or card; the entry opens the inspector, nested controls act on their own. */
export const ContainerRow = memo(function ContainerRow({ container: c, host, mode, view, columns, selecting, selected, onSelect, onOpen, onEdit, inBlock }: ContainerRowProps) {
  const t = useTranslations("containers");
  const ts = useTranslations("common.status");
  const tc = useTranslations("common.actions");
  const cmd = useContainerCommands();
  const rt = containerRuntime(c, host);
  const compact = mode === "compact";
  const condensed = view === "condensed";
  const pad = inBlock ? (compact ? "py-2.5" : "py-1") : compact ? "py-3" : "py-1.5";
  const updateActions =
    c.origin === "unraid"
      ? { onRecheck: () => void cmd.check(c.id), blockedReason: t("errors.managed-by-unraid") }
      : c.origin === "unknown"
        ? { onRecheck: () => void cmd.check(c.id), blockedReason: t("origin.unknownBody") }
        : { onUpdate: () => void cmd.update(c.id), onRecheck: () => void cmd.check(c.id) };
  const status = (
    <ToneBadge tone={RUNTIME_TONE[rt]} className="min-w-24 justify-start">
      <ToneDot tone={RUNTIME_TONE[rt]} pulse={rt === "unhealthy"} className="size-1.5" />
      {ts(rt)}
    </ToneBadge>
  );
  const origin = <OriginBadge origin={c.origin} note={c.originNote} />;
  const muted = "text-xs text-muted-foreground [overflow-wrap:anywhere]";
  const ports = <span className={cn("font-mono", muted)}>{formatPorts(c.ports) || "–"}</span>;
  const networks = <span className={muted}>{c.networks.join(", ") || "–"}</span>;
  const restart = <span className="font-mono text-xs">{c.restartPolicy}</span>;
  const uptime = <span className="text-xs text-muted-foreground">{c.startedAt && c.state === "running" ? formatRelative(c.startedAt) : "–"}</span>;
  // The image line carries the version in Compact and in the condensed table.
  const imageLine = compact || condensed || view === "cards";

  const cells: Record<string, ReactNode> = {
    name: (
      <div className="flex items-center gap-3">
        {compact && !inBlock && view !== "cards" && (
          <IconTile color={host?.color} size="md">
            {c.origin === "compose" ? <Layers /> : <Box />}
          </IconTile>
        )}
        <div className="min-w-0">
          <button type="button" onClick={() => onOpen(c.id)} aria-label={t("rowOpen", { name: c.name })} className={cn("block rounded-sm text-left font-semibold tracking-tight [overflow-wrap:anywhere] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring", compact ? "text-[14.5px]" : "text-sm")}>
            {c.name}
          </button>
          {imageLine && (
            <span className="block text-xs text-muted-foreground [overflow-wrap:anywhere]">
              {c.image.repository} · <span className="font-mono">{versionLabel(c.image)}</span>
            </span>
          )}
          {condensed && !inBlock && <HostChip host={host} className="mt-1" />}
        </div>
      </div>
    ),
    status,
    host: <HostChip host={host} />,
    origin,
    state: (
      <div className="flex flex-col items-start gap-1">
        {status}
        {origin}
      </div>
    ),
    version: <span className="font-mono text-xs [overflow-wrap:anywhere]">{versionLabel(c.image)}</span>,
    update: <UpdateIndicator name={c.name} update={c.update} actions={updateActions} />,
    load: <ContainerLoad id={c.id} memLimitMb={c.memLimitMb} unavailable={host?.status !== "online" || c.state !== "running"} />,
    ports,
    networks,
    restart,
    uptime,
    details: (
      <div className="flex max-w-56 flex-col gap-0.5">
        {ports}
        {networks}
        <span className="flex flex-wrap gap-x-2">
          {restart}
          {uptime}
        </span>
      </div>
    ),
  };

  const menu = useContainerMenu(c, { onOpen, onEdit }, false);
  const actions = <ActionsMenu groups={menu} label={`${tc("more")}: ${c.name}`} />;

  return (
    <OverviewRowView
      view={view}
      columns={columns}
      cells={cells}
      menu={menu}
      menuTitle={c.name}
      pad={pad}
      hostColor={host?.color}
      selected={selected}
      onClick={(e) => {
        if (!e.currentTarget.contains(e.target as Node) || isNestedInteractive(e.target, e.currentTarget)) return;
        onOpen(c.id);
      }}
      select={selecting ? <Checkbox checked={selected} onCheckedChange={(v) => onSelect(c.id, v === true)} aria-label={t("select.label", { name: c.name })} /> : undefined}
      actions={actions}
    />
  );
});
