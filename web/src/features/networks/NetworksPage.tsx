import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import "@xyflow/react/dist/style.css";
import "./topology.css";

import { Background, BackgroundVariant, Controls, ReactFlow, ReactFlowProvider, useReactFlow, type FitViewOptions, type NodeMouseHandler, type OnNodesChange } from "@xyflow/react";
import { AlertTriangle, ChevronsDownUp, ChevronsUpDown, FlaskConical, GitBranch, Link2, Maximize, Network, Plus, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useTopology } from "@/data/queries";
import { hostTone } from "@/domain/hosts";
import type { ContainerInfo, Host, Topology, TopologyNetwork } from "@/domain/model";
import { networkComposeOwner, removeBlock, sortNetworks } from "@/domain/networks";
import { useMotionEnabled, useTokens } from "@/shared/theme/use-design-tokens";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { useHostScope, useVisibleContainers } from "@/shared/hooks/use-host-scope";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";
import { Skeleton } from "@/shared/ui/skeleton";
import { Toggle } from "@/shared/ui/toggle";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { ConnectNetworkDialog, type ConnectRequest } from "./ConnectNetworkDialog";
import { CreateNetworkDialog } from "./CreateNetworkDialog";
import { NetworkTable } from "./NetworkTable";
import { EDGE_TYPES, NODE_TYPES } from "./node-types";
import { focusFromChanges } from "./topology-focus";
import { canvasHeight, FIT_PAD, FIT_ZOOM, fitViewport, layoutBudget, type Size } from "./topology-geometry";
import { decorate, layoutHost } from "./topology-layout";
import { TopologyDetails } from "./TopologyDetails";
import { useHostCreateCapabilities, useNetworkCapabilities } from "./use-network-capabilities";
import { useNetworkCommands } from "./use-network-commands";

/** Hosts with this many containers or fewer start with every network expanded. */
const EXPAND_ALL_UP_TO = 40;
const EMPTY: ReadonlySet<string> = new Set();
/** The canvas' own fit button shows everything, however small. */
const FIT_ALL = { padding: { top: `${FIT_PAD.top}px`, bottom: `${FIT_PAD.bottom}px`, x: `${FIT_PAD.x}px` }, maxZoom: FIT_ZOOM.max } as const satisfies FitViewOptions;
const NO_SIZE: Size = { width: 0, height: 0 };

/**
 * Client size of an element via ResizeObserver, with window resizes as a
 * fallback for embedded browsers that deliver no further observations; 0
 * until measured (happy-dom never measures). Returns the size, a callback
 * ref and the element for live reads.
 */
function useElementSize(): [Size, (el: HTMLElement | null) => void, HTMLElement | null] {
  const [el, setEl] = useState<HTMLElement | null>(null);
  const [size, setSize] = useState<Size>(NO_SIZE);
  useEffect(() => {
    if (!el) return;
    const read = () => setSize((s) => (s.width === el.clientWidth && s.height === el.clientHeight ? s : { width: el.clientWidth, height: el.clientHeight }));
    read();
    window.addEventListener("resize", read);
    const ro = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(read);
    ro?.observe(el);
    return () => {
      window.removeEventListener("resize", read);
      ro?.disconnect();
    };
  }, [el]);
  return [size, setEl, el];
}

interface Source {
  topology: Topology;
  containers: ContainerInfo[];
  hosts: Host[];
  bench: boolean;
}

/** Explicit host choice of the topology; only hosts inside the global filter are offered. */
function HostPicker({ hosts, inventory, value, onChange }: { hosts: Host[]; inventory?: Topology["inventory"]; value: string; onChange: (id: string) => void }) {
  const t = useTranslations("networks");
  const tc = useTranslations("common.host");
  return (
    <div role="radiogroup" aria-label={t("hostPicker")} className="flex flex-wrap items-center gap-1.5">
      {hosts.map((h) => {
        const on = h.id === value;
        return (
          <button
            key={h.id}
            type="button"
            role="radio"
            aria-checked={on}
            title={inventory?.[h.id]?.stale ? t("partial.host", { name: h.name }) : undefined}
            onClick={() => onChange(h.id)}
            data-glow="row"
            style={{ "--host": h.color } as React.CSSProperties}
            // Selection in the design's control colour like every other choice; the square keeps the host's own colour.
            className={cn(
              "inline-flex h-8 items-center gap-2 rounded-lg border bg-card px-3 text-sm transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
              on ? "border-control/60 bg-control/15 font-medium text-control-text" : "text-muted-foreground hover:bg-muted/40 hover:text-foreground",
            )}
          >
            <span className="size-2.5 rounded-[3px] bg-(--host)" aria-hidden />
            {h.name}
            {inventory?.[h.id]?.stale && <AlertTriangle className="size-3 text-warning" aria-hidden />}
            {h.status !== "online" && <ToneDot tone={hostTone(h)} label={tc(h.status)} className="size-1.5" />}
          </button>
        );
      })}
    </div>
  );
}

function Canvas({ source }: { source: Source }) {
  const t = useTranslations("networks");
  const tokens = useTokens();
  const motion = useMotionEnabled();
  const flow = useReactFlow();
  const [canvasSize, setCanvas, canvasEl] = useElementSize();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState("");
  const [dependencies, setDependencies] = useState(true);
  const [problems, setProblems] = useState(false);
  const [expandedByHost, setExpandedByHost] = useState<Record<string, string[]>>({});
  const [creating, setCreating] = useState(false);
  const [connectRequest, setConnectRequest] = useState<ConnectRequest | null>(null);
  const [removing, setRemoving] = useState<TopologyNetwork | null>(null);

  const requested = params.get("host");
  const availableHosts = source.hosts.filter((h) => h.status === "online" || h.id === requested || source.topology.networks.some((n) => n.hostId === h.id));
  const host = availableHosts.find((h) => h.id === requested) ?? availableHosts[0];

  // Focus lives in the URL; the host and node params change together so a host switch never keeps a foreign focus.
  const navigate = useCallback(
    (patch: { host?: string; node?: string | null }) =>
      setParams(
        (p) => {
          const next = new URLSearchParams(p);
          if (patch.host !== undefined) {
            next.set("host", patch.host);
            next.delete("node");
          }
          if (patch.node !== undefined) {
            if (patch.node) next.set("node", patch.node);
            else next.delete("node");
          }
          return next;
        },
        { replace: true },
      ),
    [setParams],
  );

  const hostContainers = useMemo(() => (host ? source.containers.filter((c) => c.hostId === host.id) : []), [source.containers, host]);
  const hostNets = useMemo(() => (host ? sortNetworks(source.topology.networks.filter((n) => n.hostId === host.id)) : []), [source.topology, host]);
  const cmd = useNetworkCommands(host?.id);
  const onlineHosts = source.hosts.filter((h) => h.status === "online");
  const createCapabilities = useHostCreateCapabilities(onlineHosts.map((h) => h.id));
  const createHosts = onlineHosts.filter((h) => createCapabilities.get(h.id)?.available);
  const createHost = createHosts.find((h) => h.id === host?.id) ?? createHosts[0];
  const connectCapabilities = useNetworkCapabilities("connectNetwork", hostNets.map((n) => n.id));
  const connectAvailable = hostNets.some((n) => !n.detailUnavailable && !n.inventory?.stale && connectCapabilities.get(n.id)?.available);
  const expandedList = host ? expandedByHost[host.id] : undefined;
  const expanded = useMemo<ReadonlySet<string>>(() => (expandedList ? new Set(expandedList) : hostContainers.length <= EXPAND_ALL_UP_TO ? new Set(hostNets.map((n) => n.id)) : EMPTY), [expandedList, hostContainers.length, hostNets]);
  const setExpanded = useCallback((ids: string[]) => host && setExpandedByHost((s) => ({ ...s, [host.id]: ids })), [host]);
  const toggleExpand = (id: string) => setExpanded(expanded.has(id) ? [...expanded].filter((x) => x !== id) : [...expanded, id]);

  // Positions depend on data, expansion and the canvas width budget only; metrics ticks, search and focus never recompute them.
  const budget = layoutBudget(canvasSize.width);
  const layout = useMemo(() => (host ? layoutHost({ topology: source.topology, containers: source.containers, host, expanded, budget }) : null), [source.topology, source.containers, host, expanded, budget]);

  // Fits on load, host switch, resize and layout changes until the user pans or zooms; "Gesamtansicht" fits again.
  const bounds = layout?.bounds;
  const moved = useRef(false);
  const fittedHost = useRef<string | undefined>(undefined);
  const height = bounds ? canvasHeight(bounds, canvasSize.width) : undefined;
  // Reads the live size: the content height just set may not have reached the observer yet.
  const fit = useCallback(
    (duration = 0) => {
      const size = canvasEl ? { width: canvasEl.clientWidth, height: canvasEl.clientHeight } : canvasSize;
      if (bounds && size.width && size.height) void flow.setViewport(fitViewport(bounds, size), { duration });
    },
    [flow, bounds, canvasSize, canvasEl],
  );
  // React Flow keeps the first onInit callback; it must reach the current fit, not the first render's.
  const latestFit = useRef(fit);
  useEffect(() => {
    latestFit.current = fit;
    if (fittedHost.current !== host?.id) {
      fittedHost.current = host?.id;
      moved.current = false;
    }
    if (!moved.current) fit();
  }, [fit, height, host?.id]);

  const requestedFocus = params.get("node");
  const focus = requestedFocus && (hostNets.some((n) => n.id === requestedFocus) || hostContainers.some((c) => c.id === requestedFocus)) ? requestedFocus : null;
  const graph = useMemo(() => (layout ? decorate(layout, { query, onlyProblems: problems, focus, showDependencies: dependencies }) : { nodes: [], edges: [] }), [layout, query, problems, focus, dependencies]);

  /** Focuses a node; a container inside a collapsed network expands that network first. */
  const focusNode = useCallback(
    (id: string | null) => {
      const home = id ? layout?.homeOf.get(id) : undefined;
      if (home && !expanded.has(home)) setExpanded([...expanded, home]);
      navigate({ node: id });
    },
    [layout, expanded, setExpanded, navigate],
  );
  const onNodesChange: OnNodesChange = useCallback(
    (changes) => {
      const next = focusFromChanges(changes);
      if (next !== undefined && next !== focus) focusNode(next);
    },
    [focus, focusNode],
  );
  const onNodeClick: NodeMouseHandler = (e, node) => {
    if (node.type === "network" && (e.target as HTMLElement).closest("[data-expand]")) toggleExpand(node.id);
  };
  const overview = () => {
    moved.current = false;
    navigate({ node: null });
    fit(motion ? 250 : 0);
  };

  if (!host) return <><InventoryNotice hosts={source.hosts} items={[]} /><EmptyState icon={<Network />} title={t("empty.title")} description={t("empty.body")} /></>;
  const online = host.status === "online";
  const removingNetwork = removing ? source.topology.networks.find((n) => n.id === removing.id) ?? removing : undefined;
  const removalBlock = removingNetwork ? removeBlock(source.topology, removingNetwork) : undefined;
  const removalReason = removalBlock ? t(`removeBlocked.${removalBlock}`, { stack: removingNetwork ? networkComposeOwner(removingNetwork) ?? "" : "", count: source.topology.attachments.filter((a) => a.networkId === removingNetwork?.id).length }) : undefined;
  const focusName = focus ? (hostNets.find((n) => n.id === focus)?.name ?? hostContainers.find((c) => c.id === focus)?.name) : undefined;

  return (
    <div className="@container/net">
      <div className="mb-3 flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <HostPicker hosts={availableHosts} inventory={source.topology.inventory} value={host.id} onChange={(id) => navigate({ host: id })} />
          {source.bench && (
            <ToneBadge tone="violet">
              <FlaskConical /> {t("bench", { count: source.containers.length })}
            </ToneBadge>
          )}
          <span className="ml-auto text-xs text-muted-foreground tabular-nums">{t("stats", { networks: layout?.stats.networks ?? 0, shown: layout?.stats.shown ?? 0, containers: layout?.stats.containers ?? 0 })}</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <InputGroup className="w-full sm:w-64">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput type="search" placeholder={t("search")} aria-label={t("searchLabel")} value={query} onChange={(e) => setQuery(e.target.value)} />
          </InputGroup>
          <Tooltip>
            <TooltipTrigger asChild>
              <Toggle variant="outline" size="sm" pressed={dependencies} onPressedChange={setDependencies} className="gap-1.5 px-3">
                <GitBranch /> {t("toggles.dependencies")}
              </Toggle>
            </TooltipTrigger>
            <TooltipContent className="max-w-64">{t("toggles.dependenciesHint")}</TooltipContent>
          </Tooltip>
          <Toggle variant="outline" size="sm" pressed={problems} onPressedChange={setProblems} className="gap-1.5 px-3">
            <AlertTriangle /> {t("toggles.problems")}
          </Toggle>
          <Button variant="outline" size="sm" onClick={() => setExpanded(hostNets.map((n) => n.id))}>
            <ChevronsUpDown data-icon="inline-start" /> {t("expandAll")}
          </Button>
          <Button variant="outline" size="sm" onClick={() => setExpanded([])}>
            <ChevronsDownUp data-icon="inline-start" /> {t("collapseAll")}
          </Button>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="outline" size="sm" onClick={overview}>
                <Maximize data-icon="inline-start" /> {t("overview")}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("overviewHint")}</TooltipContent>
          </Tooltip>
          <span className="flex-1" />
          <Button variant="outline" size="sm" disabled={!connectAvailable || !online || source.bench} title={[...connectCapabilities.values()].find((c) => c.reason)?.reason} onClick={() => setConnectRequest({ mode: "connect" })}>
            <Link2 data-icon="inline-start" /> {t("connect")}
          </Button>
          <Button size="sm" disabled={!createHost || source.bench} title={!createHost ? [...createCapabilities.values()].find((c) => c.reason)?.reason : undefined} onClick={() => setCreating(true)}>
            <Plus data-icon="inline-start" /> {t("create")}
          </Button>
        </div>
      </div>
      {source.topology.inventory?.[host.id]?.stale && <p aria-live="polite" className="mb-3 text-sm text-warning">{t("partial.host", { name: host.name })}</p>}
      {/* The page's own width decides (the sidebar counts): graph and info column about 70/30 after the gap, or stacked. */}
      <div
        className="grid items-start gap-4 @min-[72rem]/net:grid-cols-[minmax(0,7fr)_minmax(20rem,3fr)]"
        onKeyDown={(e) => {
          if (e.key === "Escape" && focus && !e.defaultPrevented) focusNode(null);
        }}
      >
        {/* Height follows the fitted content, capped by the viewport; larger graphs pan and zoom inside. */}
        <div
          ref={setCanvas}
          style={{ "--host": host.color, height } as React.CSSProperties}
          className="topology dot-grid relative max-h-[max(20rem,calc(100dvh-13rem))] min-h-80 overflow-hidden rounded-3xl border border-(--host)/30 bg-[radial-gradient(circle_at_top_left,color-mix(in_oklch,var(--host)_10%,transparent),transparent_40%)]"
          data-slot="topology-canvas"
        >
          <ReactFlow
            key={host.id}
            nodes={graph.nodes}
            edges={graph.edges}
            nodeTypes={NODE_TYPES}
            edgeTypes={EDGE_TYPES}
            colorMode={tokens.theme}
            onInit={() => !moved.current && latestFit.current()}
            // Only a real user move counts: a click on a node starts a pan gesture without moving, programmatic moves have no event.
            onMove={(event) => {
              if (event) moved.current = true;
            }}
            minZoom={0.2}
            maxZoom={2}
            nodesConnectable={false}
            nodesDraggable={false}
            multiSelectionKeyCode={null}
            selectionKeyCode={null}
            deleteKeyCode={null}
            onlyRenderVisibleElements
            onNodesChange={onNodesChange}
            onNodeClick={onNodeClick}
            onNodeDoubleClick={(_, node) => node.type === "network" && toggleExpand(node.id)}
            onPaneClick={() => focusNode(null)}
            ariaLabelConfig={{ "node.a11yDescription.default": t("nodeHint") }}
          >
            <Background variant={BackgroundVariant.Dots} gap={18} size={1.2} color="color-mix(in oklch, var(--muted-foreground) 25%, transparent)" />
            <Controls showInteractive={false} position="bottom-left" fitViewOptions={FIT_ALL} />
          </ReactFlow>
          <div className="pointer-events-none absolute top-3 left-4 text-[11px] font-semibold tracking-[0.08em] text-(--host) uppercase">{t("zone", { name: host.name, status: online ? t("online") : t("offline") })}</div>
          <ul aria-label={t("legend.label")} data-slot="topology-legend" className="pointer-events-none absolute right-3 bottom-3 flex flex-col gap-1 rounded-lg border bg-card/95 px-2.5 py-1.5 text-[10.5px] text-muted-foreground shadow-sm">
            <li className="flex items-center gap-2">
              <svg width="26" height="8" aria-hidden>
                <path d="M1 4 H25" stroke="var(--cyan)" strokeWidth="1.6" />
              </svg>
              {t("legend.membership")}
            </li>
            {dependencies && (
              <li className="flex items-center gap-2">
                <svg width="26" height="8" aria-hidden>
                  <path d="M1 4 H20" stroke="var(--foreground)" strokeWidth="1.4" strokeDasharray="4 3" />
                  <path d="M19 1 L25 4 L19 7 Z" fill="var(--foreground)" />
                </svg>
                {t("legend.dependency")}
              </li>
            )}
          </ul>
          <div className="absolute top-3 right-3 flex items-center gap-2">
            {focusName && (
              <Button size="sm" variant="secondary" className="h-7 rounded-full text-xs" onClick={() => focusNode(null)} aria-label={t("detail.clear")}>
                {t("focusOn", { name: focusName })} ✕
              </Button>
            )}
            <SimulationBadge />
          </div>
        </div>
        <div className="flex min-w-0 flex-col gap-4 @max-[72rem]/net:max-w-4xl">
          <TopologyDetails
            selected={focus}
            topology={source.topology}
            containers={source.containers}
            hosts={new Map(source.hosts.map((h) => [h.id, h]))}
            onSelect={focusNode}
            expanded={expanded}
            onToggleExpand={toggleExpand}
            onConnect={setConnectRequest}
            onRemove={setRemoving}
          />
          <NetworkTable host={host} networks={hostNets} topology={source.topology} containers={hostContainers} focus={focus} query={query} onClearQuery={() => setQuery("")} onFocus={focusNode} onConnect={setConnectRequest} onRemove={setRemoving} />
        </div>
      </div>
      {connectRequest && <ConnectNetworkDialog key={`connect:${host.id}`} request={connectRequest} onClose={() => setConnectRequest(null)} host={host} topology={source.topology} containers={source.containers} />}
      {createHost && <CreateNetworkDialog
        key={`create:${host.id}:${createHost.id}`}
        open={creating}
        onOpenChange={setCreating}
        hosts={createHosts}
        networks={source.topology.networks}
        defaultHostId={createHost.id}
        onCreated={(n) => navigate({ host: n.hostId, node: n.id })}
      />}
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("removeDialog.title", { name: removing?.name ?? "" })}
        description={removalReason ?? t("removeDialog.body", { host: host.name })}
        confirmLabel={t("removeDialog.confirm")}
        destructive
        onConfirm={async () => {
          const n = removing;
          if (!n || removalBlock || !(await cmd.remove(n.id))) return false;
          toast.success(t("removed", { name: n.name }));
          if (focus === n.id) navigate({ node: null });
        }}
      />
    </div>
  );
}

/** The topology always reflects the selected HTTP or development adapter. */
function useSource(): (Source & { isPending: boolean; isError: boolean; error?: unknown; refetch: () => void }) | null {
  const topology = useTopology();
  const containers = useVisibleContainers();
  const hosts = useHostScope();
  const none: Topology = { networks: [], attachments: [], dependencies: [] };
  if (topology.isPending || containers.isPending || hosts.isPending) return { topology: none, containers: [], hosts: [], bench: false, isPending: true, isError: false, refetch: () => {} };
  if (topology.isError) return { topology: none, containers: [], hosts: [], bench: false, isPending: false, isError: true, error: topology.error, refetch: () => void topology.refetch() };
  if (!topology.data) return null;
  return { topology: topology.data, containers: containers.visible, hosts: hosts.visible, bench: false, isPending: false, isError: false, refetch: () => void topology.refetch() };
}

function TopologySection() {
  const t = useTranslations("networks");
  const source = useSource();
  if (!source || source.isPending) {
    return (
      <div aria-busy="true" className="grid gap-4 xl:grid-cols-[minmax(0,7fr)_minmax(20rem,3fr)]">
        <Skeleton className="h-[480px] rounded-3xl" />
        <Skeleton className="h-72 rounded-xl" />
      </div>
    );
  }
  if (source.isError) return <QueryError error={source.error} onRetry={source.refetch} />;
  if (!source.hosts.length) return <><InventoryNotice hosts={source.hosts} items={[]} /><EmptyState icon={<Network />} title={t("empty.title")} description={t("empty.body")} /></>;
  return <><InventoryNotice hosts={source.hosts} items={source.topology.networks} /><Canvas source={source} /></>;
}

export default function NetworksPage() {
  const t = useTranslations("networks");
  return (
    <>
      <PageHeader title={t("title")} description={t("description")} />
      <ReactFlowProvider>
        <TopologySection />
      </ReactFlowProvider>
    </>
  );
}
