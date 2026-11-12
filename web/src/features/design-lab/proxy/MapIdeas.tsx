import { useProxyText } from "./use-proxy-text";
import { ArrowRight, Combine, Network, Waypoints } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";
import { IconTile } from "@/shared/components/tone";
import { RouteGraph } from "./RouteGraph";

import type { ProxyRoute } from "./model";

export function MapIdeas({ routes, instance, showGraph = true }: { routes: ProxyRoute[]; instance: string; showGraph?: boolean }) {
  const t = useProxyText();
  const choices = [
    { name: "ideaA", desc: "ideaADesc", icon: Waypoints, href: "#route-map" },
    { name: "ideaB", desc: "ideaBDesc", icon: Combine, href: "/networks?host=" + instance + "&proxyLayout=augmented" },
    { name: "ideaC", desc: "ideaCDesc", icon: Network, href: "/networks?host=" + instance + "&proxyLayout=switch&view=routing" },
  ] as const;
  return <div className="flex flex-col gap-5">
    <div><h2 className="text-lg font-semibold">{t("mapTitle")}</h2><p className="mt-1 text-sm text-muted-foreground">{t("mapSubtitle")}</p></div>
    <div className="grid gap-4 lg:grid-cols-3">{choices.map((idea) => <Card key={idea.name} className={idea.name === "ideaA" ? "border-primary/50 bg-primary/5" : ""}><CardHeader className="gap-3"><IconTile tone="accent"><idea.icon /></IconTile><CardTitle className="text-sm">{t(idea.name)}</CardTitle><CardDescription>{t(idea.desc)}</CardDescription></CardHeader><CardContent className="flex-1" /><CardFooter><Button size="sm" variant="outline" asChild>{idea.name === "ideaA" ? <a href={idea.href}>{t("currentIdea")}</a> : <Link to={idea.href}>{t("tryIdea")}<ArrowRight data-icon="inline-end" /></Link>}</Button></CardFooter></Card>)}</div>
    {showGraph && <div id="route-map" className="scroll-mt-5"><RouteGraph routes={routes} /></div>}
  </div>;
}
