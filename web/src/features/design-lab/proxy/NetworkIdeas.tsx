// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useProxyText } from "./use-proxy-text";
import { useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { ArrowRight, Globe, Network, Waypoints } from "lucide-react";
import { useTopology } from "@/data/queries";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { Choice } from "./controls";
import { INSTANCES, useProxy } from "./model";
import { RouteGraph } from "./RouteGraph";

export default function NetworkIdeas({ children }: { children: ReactNode }) {
  const t = useProxyText();
  const { routes } = useProxy();
  const topology = useTopology();
  const [params, setParams] = useSearchParams();
  const layout = params.get("proxyLayout") ?? "augmented";
  const host = params.get("host") ?? "atlas";
  const focus = params.get("node");
  const [view, setView] = useState(params.get("view") === "routing" ? "routing" : "network");
  const hostRoutes = routes.filter((r) => r.hostId === host);
  const attached = topology.data?.attachments.filter((a) => a.networkId === focus).map((a) => a.containerId) ?? [];
  const focused = !focus ? hostRoutes : hostRoutes.filter((r) => r.containerId === focus || (!!r.containerId && attached.includes(r.containerId)));
  return <div className="flex flex-col gap-4">
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-primary/25 bg-primary/5 p-3">
      <Choice label={t("currentIdea")} value={layout === "switch" ? "switch" : "augmented"} onChange={(next) => setParams((p) => { p.set("proxyLayout", next); return p; })} options={[{ value: "augmented", label: t("ideaB") }, { value: "switch", label: t("ideaC") }]} />
      <span className="flex-1 text-xs text-muted-foreground">{t(layout === "switch" ? "ideaCDesc" : "ideaBDesc")}</span>
      <Button variant="ghost" size="sm" asChild><Link to={"/proxy?tab=maps&instance=" + host}>{t("backToIdeas")}<ArrowRight data-icon="inline-end" /></Link></Button>
    </div>
    {layout === "switch" ? <>
      <Tabs value={view} onValueChange={setView}><TabsList className="max-w-full"><TabsTrigger value="network"><Network />{t("networkView")}</TabsTrigger><TabsTrigger value="routing"><Waypoints />{t("routingView")}</TabsTrigger></TabsList></Tabs>
      {view === "network" ? children : <><Choice label={t("instance")} value={host} onChange={(next) => setParams((p) => { p.set("host", next); p.delete("node"); return p; })} options={INSTANCES.map((p) => ({ value: p.id, label: p.name }))} /><RouteGraph routes={hostRoutes} /></>}
    </> : <>
      {children}
      <Card className="gap-4"><CardHeader className="flex flex-wrap items-start justify-between gap-3"><div><CardTitle className="text-base">{t("networkRelated")}</CardTitle><CardDescription className="mt-1">{t("networkRelatedHint")}</CardDescription></div>{focus && <Button size="sm" variant="outline" onClick={() => setParams((p) => { p.delete("node"); return p; })}>{t("clearFocus")}</Button>}</CardHeader><CardContent>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{focused.map((route) => <div key={route.id} className="flex flex-col gap-3 rounded-lg border bg-muted/20 p-4"><div className="flex min-w-0 items-center gap-2"><Globe className="size-4 shrink-0 text-cyan" /><Link className="truncate text-sm font-medium hover:underline" to={"/proxy?route=" + route.id + "&instance=" + route.proxy}>{route.domain}</Link></div><span className="truncate font-mono text-xs text-muted-foreground">{route.url}</span><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" asChild><Link to={"/proxy?route=" + route.id + "&instance=" + route.proxy}>{t("routeLink")}</Link></Button>{route.containerId && <Button size="sm" variant="ghost" onClick={() => setParams((p) => { p.set("node", route.containerId!); return p; })}><Network data-icon="inline-start" />{t("network")}</Button>}</div></div>)}</div>
        {!focused.length && <p className="py-5 text-sm text-muted-foreground">{t("empty")}</p>}
      </CardContent></Card>
    </>}
  </div>;
}
