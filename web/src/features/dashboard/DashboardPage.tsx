import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { CalendarClock, Check, Gauge, HardDrive, History, LayoutGrid, Plus, Radar, ScrollText, ShieldAlert, Siren, SlidersHorizontal, Globe } from "lucide-react";
import { useCallback, useMemo, useState, type ReactNode } from "react";
import { flushSync } from "react-dom";
import { useTranslations } from "use-intl";

import { useActionRuns, useMetrics } from "@/data/live";
import { useEvents, useSchedule, useSecurity } from "@/data/queries";
import { isHostVisible } from "@/domain/hosts";
import type { Tone } from "@/domain/model";
import { hasUpdate } from "@/domain/updates";
import { useMotionEnabled, useTokens } from "@/shared/theme/use-design-tokens";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope, useVisibleContainers, useVisibleStacks } from "@/shared/hooks/use-host-scope";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";

import { attentionItems } from "./attention";
import { CustomizeSheet } from "./CustomizeSheet";
import {
  clampSize,
  DEFAULT_WIDGET_SIZE,
  isServiceWidget,
  layoutStore,
  MAX_WIDGET_COLUMNS,
  MAX_WIDGET_ROWS,
  moveWidget,
  reorderVisible,
  resizeWidget,
  toggleWidget,
  useLayout,
  visibleWidgets,
  WIDGET_MIN,
  widgetSize,
  withTransition,
  type CoreWidgetId,
  type ServiceKind,
  type WidgetId,
  type WidgetSize,
} from "./dashboard-layout";
import { DashboardSkeleton } from "./DashboardSkeleton";
import { OpsSummary } from "./OpsSummary";
import { useAttentionText } from "./attention-text";
import { AppIcon } from "./services/app-icons";
import { SERVICE_META } from "./services/service-data";
import { ServiceConfigDialog } from "./services/ServiceConfigDialog";
import { ServiceWidgetBody } from "./services/ServiceWidgets";
import { useSortableGrid } from "./use-sortable-grid";
import { useWidgetGrid, useWidgetResize } from "./use-widget-grid";
import { AttentionWidget } from "./widgets/AttentionWidget";
import { LoadWidget } from "./widgets/LoadWidget";
import { AppsWidget, CapacityWidget, EventsWidget, ScheduleWidget, SecurityWidget } from "./widgets/SmallWidgets";
import { TimelineWidget } from "./widgets/TimelineWidget";
import { UpdateRadarWidget } from "./widgets/UpdateRadarWidget";
import { WidgetFrame } from "./widgets/WidgetFrame";

const META: Record<CoreWidgetId, { icon: ReactNode; tone: Tone }> = {
  attention: { icon: <Siren />, tone: "err" },
  updates: { icon: <Radar />, tone: "warn" },
  timeline: { icon: <History />, tone: "accent" },
  load: { icon: <Gauge />, tone: "info" },
  capacity: { icon: <HardDrive />, tone: "teal" },
  schedule: { icon: <CalendarClock />, tone: "violet" },
  security: { icon: <ShieldAlert />, tone: "orange" },
  apps: { icon: <Globe />, tone: "sky" },
  events: { icon: <ScrollText />, tone: "gray" },
};

function WidgetBodySkeleton() {
  return (
    <div className="flex h-full flex-col gap-2" aria-busy="true">
      {Array.from({ length: 5 }, (_, i) => (
        <Skeleton key={i} className="h-11 rounded-lg" />
      ))}
    </div>
  );
}

export default function DashboardPage() {
  const t = useTranslations("dashboard");
  const hosts = useHostScope();
  const stacks = useVisibleStacks();
  const containers = useVisibleContainers();
  const security = useSecurity();
  const events = useEvents();
  const schedule = useSchedule();
  const runs = useActionRuns();
  const metrics = useMetrics((s) => s?.hosts);
  const layout = useLayout();
  const animate = useMotionEnabled();
  const speed = useTokens().speed;
  const attentionText = useAttentionText();
  const [editing, setEditing] = useState(false);
  const [sheet, setSheet] = useState(false);
  const [configure, setConfigure] = useState<ServiceKind | null>(null);
  const [announce, setAnnounce] = useState("");
  const [now] = useState(() => Date.now());

  const visibleIds = useMemo(() => new Set(hosts.visible.map((h) => h.id)), [hosts.visible]);
  const images = useMemo(() => (security.data?.enabled ? security.data.images.filter((i) => isHostVisible(hosts.selection, i.hostId)) : []), [security.data, hosts.selection]);
  // Metrics tick every 2 s; disk-based items only need a coarse refresh.
  const diskKey = metrics ? Math.round(Object.values(metrics).reduce((n, m) => n + m.disk, 0)) : 0;
  const attention = useMemo(
    () => attentionItems({ hosts: hosts.visible, stacks: stacks.visible, containers: containers.visible, runs, images, metrics, now }),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- diskKey stands in for metrics
    [hosts.visible, stacks.visible, containers.visible, runs, images, now, diskKey],
  );
  const updates = stacks.visible.reduce((n, s) => n + s.services.filter((x) => hasUpdate(x.update)).length, 0) + containers.visible.filter((c) => c.origin !== "compose" && hasUpdate(c.update)).length;
  const critical = images.filter((i) => i.usage === "running").reduce((n, i) => n + i.counts.critical, 0);

  const coreLoading = hosts.isPending || stacks.isPending || containers.isPending;
  const visible = visibleWidgets(layout).filter((id) => import.meta.env.DEV || !isServiceWidget(id));
  const apply = (fn: () => void) => withTransition(() => flushSync(fn), animate);
  const title = (id: WidgetId) => (isServiceWidget(id) ? t(`services.${id}.name`) : t(`widget.${id}.title`));
  const onCommit = useCallback(
    (order: WidgetId[]) => {
      layoutStore.set((l) => reorderVisible(l, order));
      setAnnounce(t("customizeSheet.dropped"));
    },
    [t],
  );
  const grid = useSortableGrid<WidgetId>({ items: visible, enabled: editing, animate, duration: Math.round(220 * speed), onCommit });
  const { attach: attachGrid, el: gridEl, columns, style: gridStyle } = useWidgetGrid();
  const [preview, setPreview] = useState<{ id: WidgetId; size: WidgetSize } | null>(null);
  const announceSize = (id: WidgetId, size: WidgetSize) => setAnnounce(t("resize.announce", { name: title(id), w: size.w, h: size.h }));
  const commitSize = (id: WidgetId, size: WidgetSize) => {
    layoutStore.set((l) => resizeWidget(l, id, size));
    announceSize(id, widgetSize(layoutStore.get(), id));
  };
  const onResizeHandle = useWidgetResize({ columns: columns, grid: gridEl, onPreview: (id, size) => setPreview(size ? { id, size } : null), onCommit: commitSize });
  const sizeOf = (id: WidgetId) => (preview?.id === id ? preview.size : widgetSize(layout, id));
  const resize = (id: WidgetId, delta: { w?: number; h?: number } | "reset") => {
    const current = widgetSize(layout, id);
    const next = delta === "reset" ? DEFAULT_WIDGET_SIZE : clampSize(id, { w: Math.min(current.w, columns) + (delta.w ?? 0), h: current.h + (delta.h ?? 0) }, columns);
    apply(() => commitSize(id, next));
  };
  const move = (id: WidgetId, delta: number) => {
    apply(() => layoutStore.set((l) => moveWidget(l, id, delta)));
    const pos = visibleWidgets(layoutStore.get()).indexOf(id);
    setAnnounce(t("customizeSheet.moved", { name: title(id), position: pos + 1, total: visible.length }));
  };

  const body = (id: WidgetId): ReactNode => {
    if (isServiceWidget(id)) return import.meta.env.DEV ? <ServiceWidgetBody kind={id} config={layout.services[id]} onConfigure={() => setConfigure(id)} /> : null;
    switch (id) {
      case "attention":
        return <AttentionWidget items={attention} hosts={hosts.byId} />;
      case "updates":
        return <UpdateRadarWidget stacks={stacks.visible} hosts={hosts.byId} />;
      case "timeline":
        return events.isPending ? <WidgetBodySkeleton /> : events.isError ? <QueryError compact onRetry={() => void events.refetch()} /> : <TimelineWidget events={events.data} hosts={hosts.visible} now={now} />;
      case "load":
        return <LoadWidget hosts={hosts.visible} />;
      case "capacity":
        return <CapacityWidget hosts={hosts.visible} />;
      case "schedule":
        return schedule.isPending ? (
          <WidgetBodySkeleton />
        ) : schedule.isError ? (
          <QueryError compact onRetry={() => void schedule.refetch()} />
        ) : (
          <ScheduleWidget jobs={schedule.data.filter((j) => !j.hostId || visibleIds.has(j.hostId))} hosts={hosts.byId} now={now} />
        );
      case "security":
        return security.isPending ? <WidgetBodySkeleton /> : security.isError ? <QueryError compact onRetry={() => void security.refetch()} /> : <SecurityWidget security={security.data} visibleHostIds={visibleIds} />;
      case "apps":
        return <AppsWidget stacks={stacks.visible} hosts={hosts.byId} />;
      case "events":
        return events.isPending ? (
          <WidgetBodySkeleton />
        ) : events.isError ? (
          <QueryError compact onRetry={() => void events.refetch()} />
        ) : (
          <EventsWidget events={events.data.filter((e) => !e.hostId || visibleIds.has(e.hostId))} hosts={hosts.byId} />
        );
    }
  };

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <>
            {editing && (
              <Button variant="outline" onClick={() => setSheet(true)}>
                <Plus data-icon="inline-start" /> {t("addWidget")}
              </Button>
            )}
            <Button variant={editing ? "default" : "outline"} onClick={() => setEditing((e) => !e)} aria-pressed={editing} className="group">
              {editing ? <Check data-icon="inline-start" /> : <SlidersHorizontal data-icon="inline-start" className="tilt" />}
              {editing ? t("done") : t("customize")}
            </Button>
          </>
        }
      />
      <InventoryNotice hosts={hosts.visible} items={[...stacks.visible, ...containers.visible, ...Object.values(metrics ?? {})]} />
      {coreLoading ? (
        <DashboardSkeleton sizes={visible.map((id) => widgetSize(layout, id))} />
      ) : stacks.isError || containers.isError ? (
        <QueryError onRetry={() => void Promise.all([stacks.refetch(), containers.refetch()])} />
      ) : (
        <>
          <OpsSummary
            hosts={hosts.visible}
            allHostsCount={hosts.all.length}
            stacks={stacks.visible}
            containers={containers.visible}
            attention={attention}
            attentionText={attentionText}
            updates={updates}
            critical={critical}
            runningActions={runs.filter((r) => r.status === "running").length}
            editing={editing}
            animate={animate}
            duration={Math.round(220 * speed)}
          />
          {editing && <p className="mb-3 rounded-xl border border-dashed border-primary/40 bg-primary/6 px-3 py-2 text-xs text-muted-foreground">{t("editHint")}</p>}
          <p className="sr-only" aria-live="polite">
            {announce}
          </p>
          {visible.length === 0 ? (
            <EmptyState icon={<LayoutGrid />} title={t("allHidden")} action={<Button onClick={() => setSheet(true)}>{t("chooseWidgets")}</Button>} />
          ) : (
            <div ref={attachGrid} className="grid" style={gridStyle} data-slot="widget-grid" data-columns={columns} data-editing={editing || undefined}>
              {grid.order.map((id, i) => (
                <WidgetFrame
                  key={id}
                  id={id}
                  title={title(id)}
                  description={isServiceWidget(id) ? t(`services.${id}.desc`) : t(`widget.${id}.desc`)}
                  icon={isServiceWidget(id) ? <AppIcon kind={id} /> : META[id].icon}
                  tone={isServiceWidget(id) ? undefined : META[id].tone}
                  accent={isServiceWidget(id) ? SERVICE_META[id].color : undefined}
                  slotRef={grid.slotRef(id)}
                  cardRef={grid.cardRef(id)}
                  chrome={{
                    editing,
                    first: i === 0,
                    last: i === grid.order.length - 1,
                    dragging: grid.dragging === id,
                    dropTarget: !!grid.dragging && grid.dragging !== id,
                    onPointerDown: (e) => grid.onPointerDown(id, e),
                    onMove: (delta) => move(id, delta),
                    onHide: () => apply(() => layoutStore.set((l) => toggleWidget(l, id))),
                    onConfigure: import.meta.env.DEV && isServiceWidget(id) ? () => setConfigure(id) : undefined,
                    size: sizeOf(id),
                    columns: columns,
                    limits: { minW: WIDGET_MIN[id].w, maxW: Math.min(MAX_WIDGET_COLUMNS, columns), minH: WIDGET_MIN[id].h, maxH: MAX_WIDGET_ROWS },
                    resizing: preview?.id === id,
                    onResize: (delta) => resize(id, delta),
                    onResizeHandle: (e) => onResizeHandle(id, widgetSize(layout, id), e),
                  }}
                >
                  {body(id)}
                </WidgetFrame>
              ))}
            </div>
          )}
        </>
      )}
      <CustomizeSheet open={sheet} onOpenChange={setSheet} animate={animate} meta={META} onConfigure={import.meta.env.DEV ? setConfigure : undefined} />
      {import.meta.env.DEV && <ServiceConfigDialog kind={configure} onOpenChange={(o) => !o && setConfigure(null)} />}
    </>
  );
}
