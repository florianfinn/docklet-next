import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { useCapability } from "@/data/use-capability";
import { LogDeliveryNotice, LogMessage, LogSource } from "@/features/stacks/logs/log-presentation";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { ArrowLeft, Boxes, Cpu, Layers, Lock, Package, Pencil, Play, RotateCw, Square } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { useTranslations } from "use-intl";

import { useContainerLogs, useMetrics } from "@/data/live";
import { useContainers } from "@/data/queries";
import { formatTime } from "@/domain/format";
import type { ContainerInfo, Host } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE } from "@/domain/status";
import { imageHref } from "@/features/images/image-links";
import { HostChip } from "@/shared/components/host-chip";
import { ContainerMemory } from "@/shared/components/container-memory";
import { Meter } from "@/shared/components/meter";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { ToneBadge } from "@/shared/components/tone";
import { UpdatePanel } from "@/shared/components/updates/update-panel";
import { HelpTarget } from "@/shared/help/help-tooltip";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";
import { Toggle } from "@/shared/ui/toggle";

import { ContainerTerminalTabs } from "./ContainerTerminalTabs";
import { ContainerFacts, ContainerProvenance } from "./ContainerFacts";
import { useContainerRestart } from "./container-restart";
import { OriginBadge } from "./OriginBadge";
import { RecreateDialog } from "./RecreateDialog";

const LEVEL_CLASS = { info: "", debug: "text-muted-foreground", warn: "bg-warning/8 text-warning", error: "bg-destructive/10 text-destructive" } as const;

/** Latest lines of this container only; the stack page keeps the full multiplexed viewer. */
function ContainerLogs({ c }: { c: ContainerInfo }) {
  const t = useTranslations("containers.page");
  const [paused, setPaused] = useState(false);
  const lines = useContainerLogs(c.id, paused);
  const shown = useMemo(() => lines.slice(-120), [lines]);
  return (
    <Card className="gap-0 overflow-hidden py-0" data-slot="container-logs">
      <CardHeader className="flex flex-row flex-wrap items-center gap-2 border-b px-4 py-3">
        <CardTitle className="text-sm">{t("logs")}</CardTitle>
        <SimulationBadge />
        <span className="flex-1" />
        <Toggle size="sm" variant="outline" pressed={paused} onPressedChange={setPaused} aria-label={paused ? t("logsResume") : t("logsPause")}>
          {paused ? <Play /> : <Square />}
        </Toggle>
        {c.stackId && (
          <Button size="sm" variant="outline" asChild>
            <Link to={`/stacks/${c.stackId}?tab=logs${c.serviceName ? `&service=${encodeURIComponent(c.serviceName)}` : ""}`}>{t("logsInStack")}</Link>
          </Button>
        )}
      </CardHeader>
      <LogDeliveryNotice lines={lines} paused={paused} />
      <div role="log" tabIndex={0} aria-label={t("logs")} aria-live={paused ? "off" : "polite"} className="h-80 overflow-auto bg-code py-2 font-mono text-[12px] leading-relaxed">
        {shown.length === 0 && <p className="px-4 py-6 text-center text-muted-foreground">{t("logsEmpty")}</p>}
        {shown.map((l) => (
          <div key={l.id} className={cn("flex gap-3 px-3 whitespace-pre-wrap", LEVEL_CLASS[l.level])}>
            <span className="shrink-0 text-muted-foreground/70 select-none">{formatTime(l.ts, true)}</span>
            <span className="w-28 shrink-0 truncate text-muted-foreground select-none"><LogSource line={l} /></span>
            <span className="min-w-0 break-words"><LogMessage line={l} /></span>
          </div>
        ))}
      </div>
    </Card>
  );
}

/** Start/stop/restart in context; edit follows the origin: stack editor, recreate dialog or locked. */
function ContainerActions({ c, onEdit }: { c: ContainerInfo; onEdit: () => void }) {
  const cmd = useContainerCommands();
  const t = useTranslations("containers.inspector");
  const tm = useTranslations("containers.menu");
  const to = useTranslations("containers.origin");
  const command = useCapability("runContainerCommand", { kind: "container", id: c.id });
  const edit = useCapability("recreateContainer", { kind: "container", id: c.id });
  const restart = useContainerRestart();
  return (
    <>
      {c.state === "running" ? (
        <>
          <HelpTarget topic="action.stop">
            <Button size="sm" variant="outline" disabled={!command.available || !!c.inventory?.stale} title={command.reason} onClick={() => void cmd.run(c.id, "stop")}>
              <Square data-icon="inline-start" /> {tm("stop")}
            </Button>
          </HelpTarget>
          <HelpTarget topic="action.restart">
            <Button size="sm" variant="outline" className="group" disabled={!command.available || !!c.inventory?.stale} title={command.reason} onClick={() => restart([c])}>
              <RotateCw data-icon="inline-start" className="spin-on-hover" /> {tm("restart")}
            </Button>
          </HelpTarget>
        </>
      ) : (
        <Button size="sm" variant="outline" disabled={!command.available || !!c.inventory?.stale} title={command.reason} onClick={() => void cmd.run(c.id, "start")}>
          <Play data-icon="inline-start" /> {tm("start")}
        </Button>
      )}
      {c.origin === "standalone" && (
        <HelpTarget topic="container.recreate">
          <Button size="sm" disabled={!edit.available || !!c.inventory?.stale} title={edit.reason} onClick={onEdit}>
            <Pencil data-icon="inline-start" /> {t("edit")}
          </Button>
        </HelpTarget>
      )}
      {c.origin === "compose" && c.stackId && (
        <Button size="sm" asChild>
          <Link to={`/stacks/${c.stackId}?tab=editor`}>
            <Layers data-icon="inline-start" /> {t("openStack")}
          </Link>
        </Button>
      )}
      {(c.origin === "unraid" || c.origin === "unknown") && (
        <Button size="sm" variant="secondary" disabled title={edit.reason ?? to(`${c.origin}Body`)}>
          <Lock data-icon="inline-start" /> {t("editBlocked")}
        </Button>
      )}
    </>
  );
}

function Body({ c, host, onEdit }: { c: ContainerInfo; host: Host | undefined; onEdit: () => void }) {
  const t = useTranslations("containers.page");
  const ti = useTranslations("containers.inspector");
  const ts = useTranslations("common.status");
  const m = useMetrics((s) => s?.containers[c.id]);
  const rt = containerRuntime(c, host);
  return (
    <>
      <PageHeader
        eyebrow={
          <Link to="/containers" className="mb-1 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
            <ArrowLeft className="size-3.5" aria-hidden /> {t("back")}
          </Link>
        }
        title={c.name}
        description={
          <span className="flex flex-wrap items-center gap-2 pt-1">
            <ToneBadge tone={RUNTIME_TONE[rt]}>{ts(rt)}</ToneBadge>
            <OriginBadge origin={c.origin} source={c.originSource} note={c.originNote} />
            <HostChip host={host} />
            {c.stackId && (
              <Link to={`/stacks/${c.stackId}`} className="text-primary-text underline-offset-4 hover:underline">
                {t("stack", { service: c.serviceName ?? c.name })}
              </Link>
            )}
          </span>
        }
        actions={<ContainerActions c={c} onEdit={onEdit} />}
      />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="flex min-w-0 flex-col gap-4">
          <ContainerProvenance c={c} />
          <Card className="gap-3 py-4">
            <CardHeader className="flex flex-row items-center justify-between px-4">
              <CardTitle className="text-sm">{ti("resources")}</CardTitle>
              <SimulationBadge />
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-4 px-4">
              <Meter label="CPU" icon={<Cpu />} value={m?.cpu ?? 0} color={host?.color} />
              <ContainerMemory usedMb={m?.memMb} limitMb={c.memLimitMb} color={host?.color} />
            </CardContent>
          </Card>
          <Card className="gap-0 overflow-hidden py-0">
            <UpdatePanel
              update={c.update}
              actions={
                {}
              }
            />
          </Card>
        </div>
        <Card className="min-w-0 gap-3 py-4">
          <CardHeader className="flex flex-row flex-wrap items-center gap-2 px-4">
            <CardTitle className="text-sm">{t("details")}</CardTitle>
            <span className="flex-1" />
            <Button size="sm" variant="ghost" asChild>
              <Link to={imageHref(c)}>
                <Package data-icon="inline-start" /> {t("image")}
              </Link>
            </Button>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 px-4">
            <ContainerFacts c={c} host={host} />
            <p className="text-xs text-muted-foreground">{ti("envMasked")}</p>
          </CardContent>
        </Card>
      </div>
      <div className="mt-4">
        <ContainerTerminalTabs key={c.id} c={c} host={host}><ContainerLogs c={c} /></ContainerTerminalTabs>
      </div>
    </>
  );
}

/** Lightweight page of one container: status, resources, update, details, logs and its own actions. */
export default function ContainerPage() {
  const t = useTranslations("containers.page");
  const { containerId = "" } = useParams();
  const containers = useContainers();
  const hosts = useHostScope();
  const c = containers.data?.find((x) => x.id === containerId);
  // The page owns the recreate run at a stable position: the old ID leaves the
  // list before the run settles, and the run must survive that to navigate.
  const [recreating, setRecreating] = useState<ContainerInfo>();
  return <>{content()}<RecreateDialog container={recreating} open={!!recreating} onOpenChange={(open) => { if (!open) setRecreating(undefined); }} /></>;

  function content() {
    if (containers.isPending || hosts.isPending)
      return (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-9 w-64" />
          <Skeleton className="h-64 w-full rounded-2xl" />
        </div>
      );
    if (containers.isError) return <QueryError error={containers.error} onRetry={() => void containers.refetch()} />;
    if (!c)
      return (
        <EmptyState
          icon={<Boxes />}
          title={t("missing")}
          action={
            <Button variant="outline" asChild>
              <Link to="/containers">{t("back")}</Link>
            </Button>
          }
        />
      );
    return <><InventoryNotice hosts={hosts.all.filter((host) => host.id === c.hostId)} items={[c]} /><Body key={c.id} c={c} host={hosts.byId.get(c.hostId)} onEdit={() => { if (c.origin === "standalone") setRecreating(c); }} /></>;
  }
}
