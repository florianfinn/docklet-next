// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { ExternalLink, Globe, LockKeyhole } from "lucide-react";
import { useHosts } from "@/data/queries";
import { ActionsMenu } from "@/shared/components/action-menu";
import { OverviewTable, OverviewRowView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import type { FitView } from "@/shared/lib/fit-view";
import { isNestedInteractive } from "@/shared/lib/row-navigation";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { RouteStatus } from "./controls";
import type { RouteActionsProps } from "./RouteActions";
import { useRouteMenu } from "./use-route-menu";
import { routeName, matchingCertificates, useProxy, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
function ManagerRow({ route, actions, view, columns }: { route: ProxyRoute; actions: RouteActionsProps; view: FitView; columns: OverviewColumn[] }) {
  const t = useProxyText();
  const { certificates } = useProxy();
  const hosts = useHosts();
  const host = hosts.data?.find((h) => h.id === route.hostId);
  const cert = matchingCertificates(route, certificates)[0];
  const menu = useRouteMenu(actions);
  const open = route.source === "file" ? actions.onEdit : actions.onInspect;
  return <OverviewRowView view={view} columns={columns} pad="py-4" hostColor={host?.color} menu={menu} menuTitle={route.domain}
    onClick={(e) => { if (e.defaultPrevented || e.button !== 0 || !e.currentTarget.contains(e.target as Node) || isNestedInteractive(e.target, e.currentTarget) || window.getSelection()?.toString()) return; open(); }}
    actions={<ActionsMenu groups={menu} label={t("moreActions") + " · " + route.domain} />}
    cells={{
      name: <button className="flex items-center gap-3 rounded-sm text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring" onClick={open}><IconTile color={host?.color}><Globe /></IconTile><span><strong className="block text-sm">{route.domain}</strong><span className="mt-1 block text-xs text-muted-foreground">{routeName(route)} · {route.path || "/"}{route.stackId && " · " + route.stackId}</span></span></button>,
      target: <a href={route.url} target="_blank" rel="noopener noreferrer" className="group/target inline-flex max-w-full items-center gap-1.5 rounded-sm font-mono text-xs text-muted-foreground transition-colors hover:text-primary-text focus-visible:ring-2 focus-visible:ring-ring" aria-label={t("openTarget") + " · " + route.url}><span className="break-all underline-offset-4 group-hover/target:underline">{route.url}</span><ExternalLink className="size-3 shrink-0 transition-transform group-hover/target:-translate-y-0.5 group-hover/target:translate-x-0.5" /></a>,
      tls: <span className="flex items-center gap-2 text-xs">{route.tls !== "off" && <LockKeyhole className="size-3.5 text-success" />}{route.tls === "off" ? t("off") : cert?.name ?? t(route.tls === "auto" ? "requestPlanned" : "noCert")}</span>,
      source: <ToneBadge tone={route.source === "file" ? "gray" : "cyan"}>{t(route.source)}</ToneBadge>,
      status: <RouteStatus enabled={route.enabled} />,
    }} />;
}
export function ManagerRoutes({ routes, actions }: { routes: ProxyRoute[]; actions: (route: ProxyRoute) => RouteActionsProps }) {
  const t = useProxyText();
  const columns: OverviewColumn[] = [ { id: "name", label: t("domain"), className: "min-w-48 pl-3" }, ...(["target", "tls", "source", "status"] as const).map((id) => ({ id, label: t(id) })) ];
  return <OverviewTable fitKey={routes.map((r) => r.id).join(",")} columns={() => columns} dense={false} selecting={false} actionsLabel={t("actions")} listLabel={t("proxyHosts")}>{(view, cols) => routes.map((route) => <ManagerRow key={route.id} route={route} actions={actions(route)} view={view} columns={cols} />)}</OverviewTable>;
}
