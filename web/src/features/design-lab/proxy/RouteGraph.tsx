// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import "@xyflow/react/dist/style.css";
import { instanceKind } from "./route-text";
import { useProxyText } from "./use-proxy-text";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { Background, Controls, Handle, MarkerType, Position, ReactFlow, type Edge, type Node, type NodeProps } from "@xyflow/react";
import { Box, Globe, SlidersHorizontal, Waypoints } from "lucide-react";
import { useTokens } from "@/shared/theme/use-design-tokens";
import { MiddlewareHelp } from "./MiddlewareHelp";
import { Choice } from "./controls";
import { useProxy, type ProxyRoute } from "./model";

type RouteNodeData = { title: string; subtitle: string; kind: number; routeId: string; proxy: string; enabled: boolean; middlewares: string[] };
type RouteNode = Node<RouteNodeData, "route">;
const ICONS = [Globe, Waypoints, SlidersHorizontal, Box];
function RoutingNode({ data }: NodeProps<RouteNode>) {
  const Icon = ICONS[data.kind] ?? Box;
  return <div className={"w-56 rounded-xl border bg-card px-4 py-3 shadow-sm " + (data.enabled ? "border-primary/30" : "opacity-50")}>
    <Handle type="target" position={Position.Left} className="!size-1.5 !border-0 !bg-primary" />
    <div className="flex items-center gap-3"><Icon className={"size-4 shrink-0 " + (data.kind === 0 ? "text-cyan" : "text-primary-text")} /><div className="min-w-0"><p className="truncate text-xs font-semibold">{data.kind === 2 && data.middlewares.length ? data.middlewares.map((name, index) => <span key={name}>{index > 0 && " → "}<MiddlewareHelp name={name} /></span>) : data.title}</p><p className="mt-1 truncate text-[10px] text-muted-foreground">{data.subtitle}</p></div></div>
    <Handle type="source" position={Position.Right} className="!size-1.5 !border-0 !bg-primary" />
  </div>;
}
const NODE_TYPES = { route: RoutingNode };

export function RouteGraph({ routes }: { routes: ProxyRoute[] }) {
  const t = useProxyText();
  const tokens = useTokens();
  const { connections } = useProxy();
  const navigate = useNavigate();
  const [filter, setFilter] = useState("all");
  const chosen = routes.some((r) => r.id === filter) ? filter : "all";
  const graph = useMemo(() => {
    const nodes: RouteNode[] = [];
    const edges: Edge[] = [];
    routes.filter((r) => chosen === "all" || r.id === chosen).forEach((route, row) => {
      const titles = [route.domain, connections[route.proxy]?.container ?? (instanceKind(route.proxy) === "caddy" ? "Caddy" : "Traefik") + " · " + route.proxy, route.middlewares.join(" → ") || t("noMiddleware"), route.url.replace(/^https?:\/\//, "")];
      const subtitles = [route.path || "/", route.tls === "off" ? "web · :80" : "websecure · :443", route.middlewares.length ? t("middlewares") : t("target"), route.stackId ?? t("externalTarget")];
      titles.forEach((title, col) => {
        const id = route.id + "-" + col;
        nodes.push({ id, type: "route", position: { x: col * 290, y: row * 105 }, data: { title, subtitle: subtitles[col] ?? "", kind: col, routeId: route.id, proxy: route.proxy, enabled: route.enabled, middlewares: route.middlewares } });
        if (col) edges.push({ id: id + "-edge", source: route.id + "-" + (col - 1), target: id, type: "smoothstep", markerEnd: { type: MarkerType.ArrowClosed, color: "var(--primary)" }, style: { stroke: "var(--primary)", strokeWidth: 1.5, opacity: route.enabled ? 0.7 : 0.25, strokeDasharray: route.enabled ? undefined : "4 4" } });
      });
    });
    return { nodes, edges };
  }, [routes, chosen, t, connections]);
  return <div className="overflow-hidden rounded-xl border bg-card">
    <div className="flex flex-wrap items-center justify-between gap-3 border-b p-4"><p className="text-xs text-muted-foreground">{t("graphHint")}</p><Choice label={t("routes")} value={chosen} onChange={setFilter} options={[{ value: "all", label: t("allRoutes") }, ...routes.map((r) => ({ value: r.id, label: r.domain }))]} /></div>
    <div className="grid grid-cols-4 gap-3 border-b px-5 py-3 text-center text-xs text-muted-foreground">{(["domainsColumn", "proxyColumn", "middlewareColumn", "targetsColumn"] as const).map((key) => <span key={key}>{t(key)}</span>)}</div>
    {graph.nodes.length ? <div className="dot-grid min-w-0" style={{ height: Math.max(360, Math.min(860, graph.nodes.length / 4 * 100 + 60)) }} aria-label={t("routingView")}>
      <ReactFlow key={chosen + routes.length} nodes={graph.nodes} edges={graph.edges} nodeTypes={NODE_TYPES} colorMode={tokens.theme} fitView fitViewOptions={{ padding: 0.08, maxZoom: 1 }} minZoom={0.25} maxZoom={1.8} nodesDraggable={false} nodesConnectable={false} deleteKeyCode={null} onNodeClick={(_, node) => navigate("/proxy?route=" + node.data.routeId + "&instance=" + node.data.proxy)}>
        <Background gap={22} size={1} /><Controls showInteractive={false} />
      </ReactFlow>
    </div> : <p className="p-10 text-center text-sm text-muted-foreground">{t("empty")}</p>}
  </div>;
}
