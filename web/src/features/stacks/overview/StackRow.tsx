import { useAdapter } from "@/data/use-adapter";
import { Globe, Layers, TriangleAlert } from "lucide-react";
import { memo, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useMetrics } from "@/data/live";
import { formatPorts, formatRelative } from "@/domain/format";
import type { Host, Stack } from "@/domain/model";
import { LoadView } from "@/shared/components/container-load";
import { HostChip } from "@/shared/components/host-chip";
import { LabelChips } from "@/shared/components/label-chips";
import { OverviewRowView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { ServicesHover } from "@/shared/components/services-hover";
import { IconTile } from "@/shared/components/tone";
import { StackUpdateIndicator } from "@/shared/components/updates/update-indicator";
import { useAdapterText } from "@/data/use-adapter-text";
import type { FitView } from "@/shared/lib/fit-view";
import { useRowNavigation } from "@/shared/lib/row-navigation";
import { cn } from "@/shared/lib/utils";
import type { OverviewMode } from "@/shared/preferences/preferences-store";
import { Checkbox } from "@/shared/ui/checkbox";

import { StackStatusBadge } from "../StackStatusBadge";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { stackContainerIds } from "./stack-filters";
import { useStackMenu } from "./stack-menu";
import { StackRowActions } from "./StackRowActions";

interface StackRowProps {
  stack: Stack;
  host: Host | undefined;
  mode: OverviewMode;
  view: FitView;
  columns: OverviewColumn[];
  selecting: boolean;
  selected: boolean;
  onSelect: (id: string, on: boolean) => void;
  /** Sum of the stack's container memory limits, for the RAM ring. */
  memLimitMb: number;
  /** Row inside a host block: the host is the block heading. */
  inBlock?: boolean;
}

/** Live CPU and memory of all running services; null when none is measured. */
function useStackLoad(stack: Stack): { cpu: number; memMb: number } | null {
  const key = useMetrics((snap) => {
    let cpu = 0;
    let mem = 0;
    for (const id of stackContainerIds(stack)) {
      const m = snap?.containers[id];
      if (!m || m.memMb === 0) continue;
      cpu += m.cpu;
      mem += m.memMb;
    }
    return mem ? `${Math.round(cpu * 10) / 10}|${mem}` : "";
  });
  if (!key) return null;
  const [cpu = "0", mem = "0"] = key.split("|");
  return { cpu: Number(cpu), memMb: Number(mem) };
}

/** One stack as table row or card; the whole entry navigates, nested controls keep their own behaviour. */
export const StackRow = memo(function StackRow({ stack, host, mode, view, columns, selecting, selected, onSelect, memLimitMb, inBlock }: StackRowProps) {
  const t = useTranslations("stacks");
  const dt = useAdapterText();
  const rowNav = useRowNavigation();
  const cmd = useStackCommands();
  const href = `/stacks/${stack.id}`;
  const menu = useStackMenu(stack, false);
  const load = useStackLoad(stack);
  const scheduleText = useScheduleText();
  const compact = mode === "compact";
  const condensed = view === "condensed";
  const pad = inBlock ? (compact ? "py-2.5" : "py-1") : compact ? "py-3.5" : "py-1.5";
  const updateActions = {
    onUpdate: () => void cmd.installUpdates(stack.id),
    onRecheck: () => void cmd.checkUpdates(stack.id),
    busy: !!stack.busyActionId,
  };
  const online = host?.status === "online";
  const status = <StackStatusBadge stack={stack} host={host} />;
  const services = <ServicesHover services={stack.services} host={host} name={stack.name} />;
  const ports = <span className="font-mono text-xs text-muted-foreground [overflow-wrap:anywhere]">{formatPorts(stack.services.flatMap((s) => s.ports)) || "–"}</span>;
  const live = useAdapter().kind === "http";
  const policyText = useTranslations("common.stackUpdates");
  const auto = live ? <span className="text-xs text-muted-foreground">{policyText("unknown")}</span> : <span className="text-xs">{stack.autoUpdate.mode === "check" ? t("auto.check") : t("auto.scheduled", { window: scheduleText.summary(stack.autoUpdate.schedule) })}</span>;
  const deploy = <span className="text-xs text-muted-foreground">{stack.lastDeployAt ? formatRelative(stack.lastDeployAt) : "–"}</span>;
  const showTile = compact && !inBlock && view !== "cards";

  const cells: Record<string, ReactNode> = {
    name: (
      <div className="flex items-center gap-3">
        {showTile && (
          <IconTile color={host?.color} size="md" className="tilt">
            <Layers />
          </IconTile>
        )}
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <Link
              to={href}
              className={cn("rounded-sm font-semibold tracking-tight [overflow-wrap:anywhere] outline-none hover:underline focus-visible:underline focus-visible:ring-2 focus-visible:ring-ring", compact ? "text-[15px]" : "text-sm")}
              aria-label={t("rowOpen", { name: stack.name })}
            >
              {stack.name}
            </Link>
            {stack.definitionHint === "definition-invalid" && <span title={t("definitionHint.definition-invalid")}><TriangleAlert role="img" aria-label={t("definitionHint.definition-invalid")} className="size-4 text-warning" /></span>}
            {(!compact || inBlock) && <LabelChips labels={stack.labels} size="sm" />}
          </div>
          {((compact && !inBlock) || stack.url || (condensed && !inBlock)) && (
            <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              {condensed && !inBlock && <HostChip host={host} />}
              {compact && !inBlock && stack.description && <span className="text-xs text-muted-foreground">{dt(stack.description)}</span>}
              {compact && !inBlock && <LabelChips labels={stack.labels} size="sm" />}
              {stack.url && (
                <span className="inline-flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                  <Globe className="size-3 shrink-0 text-info" aria-hidden />
                  <span className="[overflow-wrap:anywhere]">{stack.url.replace(/^https?:\/\//, "")}</span>
                </span>
              )}
            </div>
          )}
        </div>
      </div>
    ),
    status,
    host: <HostChip host={host} />,
    services,
    state: (
      <div className="flex flex-col items-start gap-1">
        {status}
        {services}
      </div>
    ),
    updates: <StackUpdateIndicator name={stack.name} services={stack.services} actions={updateActions} />,
    load: <LoadView values={online ? load : null} memLimitMb={memLimitMb} />,
    ports,
    auto,
    deploy,
    details: (
      <div className="flex max-w-56 flex-col gap-0.5">
        {ports}
        {auto}
        {deploy}
      </div>
    ),
  };

  return (
    <OverviewRowView
      view={view}
      columns={columns}
      cells={cells}
      menu={menu}
      menuTitle={stack.name}
      pad={pad}
      hostColor={host?.color}
      selected={selected}
      busy={!!stack.busyActionId}
      onClick={rowNav(href).onClick}
      select={selecting ? <Checkbox checked={selected} onCheckedChange={(v) => onSelect(stack.id, v === true)} aria-label={t("select.label", { name: stack.name })} /> : undefined}
      actions={<StackRowActions stack={stack} menu={menu} />}
    />
  );
});
