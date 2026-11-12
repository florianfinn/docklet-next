import { useProxyText } from "./use-proxy-text";
import { useState } from "react";
import { Globe, Plus, Waypoints } from "lucide-react";
import { Link } from "react-router";
import { useContainers } from "@/data/queries";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";
import { ToneBadge } from "@/shared/components/tone";
import { RouteEditor } from "./RouteEditor";

import { newRoute, useProxy, type ProxyRoute } from "./model";

export default function ProxyContext({ containerId, stackId, hostId }: { containerId?: string; stackId?: string; hostId: string }) {
  const t = useProxyText();
  const { routes, instances, connections, instance: activeInstance } = useProxy();
  const containers = useContainers();
  const [editing, setEditing] = useState<ProxyRoute | null>(null);
  const list = routes.filter((r) => containerId ? r.containerId === containerId : r.stackId === stackId);
  const instance = list[0]?.proxy ?? instances.find((p) => p.id === activeInstance && p.hostId === hostId && connections[p.id])?.id ?? instances.find((p) => p.hostId === hostId)?.id ?? "atlas";
  function add() {
    const target = containers.data?.find((c) => containerId ? c.id === containerId : c.stackId === stackId && c.ports.length > 0);
    const route = newRoute(instance);
    setEditing({ ...route, hostId, ...(target ? { containerId: target.id, stackId: target.stackId, url: "http://" + (target.serviceName ?? target.name) + ":" + (target.ports[0]?.container ?? "80") } : { stackId }) });
  }
  return <Card className="gap-4" data-slot="proxy-context">
    <CardHeader><CardTitle className="flex items-center gap-2 text-sm"><Waypoints className="size-4 text-primary-text" />{t("contextTitle")}</CardTitle><CardDescription>{t("contextSubtitle")}</CardDescription></CardHeader>
    <CardContent className="flex flex-col gap-2">{list.map((route) => <Link key={route.id} to={"/proxy?route=" + route.id + "&instance=" + route.proxy} className="flex flex-wrap items-center gap-3 rounded-lg border bg-muted/20 p-3 hover:border-primary/50"><Globe className="size-4 shrink-0 text-cyan" /><span className="min-w-0 flex-1 truncate text-sm font-medium">{route.domain}{route.path}</span><ToneBadge tone={route.tls === "off" ? "gray" : "ok"}>{route.tls === "off" ? t("httpProtocol") : t("httpsProtocol")}</ToneBadge></Link>)}{!list.length && <p className="text-sm text-muted-foreground">{t("contextEmpty")}</p>}</CardContent>
    <CardFooter className="flex-wrap gap-2"><Button size="sm" onClick={add}><Plus data-icon="inline-start" />{t("addRoute")}</Button><Button size="sm" variant="outline" asChild><Link to={"/proxy?instance=" + instance + "&" + (containerId ? "container=" + containerId : "stack=" + stackId)}>{t("routes")}</Link></Button></CardFooter>
    {editing && <RouteEditor initial={editing} creating onClose={() => setEditing(null)} />}
  </Card>;
}
