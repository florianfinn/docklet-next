import { useState } from "react";
import { stringify } from "yaml";
import { FileCode2 } from "lucide-react";
import { ProxyTabs } from "./ProxyTabs";
import { Tabs, TabsContent } from "@/shared/ui/tabs";
import { Detail } from "./controls";
import { MiddlewareHelp } from "./MiddlewareHelp";
import { routeRule, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";

export function LabelDetails({ route }: { route: ProxyRoute }) {
  const t = useProxyText();
  const [view, setView] = useState("summary");
  const prefix = "traefik.http.routers." + route.id;
  const url = new URL(route.url);
  const labels = {
    "traefik.enable": "true", [prefix + ".rule"]: routeRule(route),
    [prefix + ".entrypoints"]: route.tls === "off" ? "web" : "websecure",
    ...(route.tls === "auto" ? { [prefix + ".tls.certresolver"]: route.resolver } : {}),
    ...(route.middlewares.length ? { [prefix + ".middlewares"]: route.middlewares.map((m) => m + "@file").join(",") } : {}),
    ["traefik.http.services." + route.id + ".loadbalancer.server.port"]: url.port || (url.protocol === "https:" ? "443" : "80"),
  };
  return <section className="mt-5 min-w-0 rounded-xl border border-cyan/25 bg-cyan/5 p-4"><h3 className="mb-3 flex items-center gap-2 text-sm font-semibold"><FileCode2 className="size-4 text-cyan" />{t("labelsTitle")}</h3><Tabs value={view} onValueChange={setView}><ProxyTabs options={[{ value: "summary", label: t("labelsSummary") }, { value: "raw", label: t("labelsRaw") }]} /><TabsContent value="summary"><dl><Detail label={t("labelMatch")}>{route.domain}</Detail><Detail label={t("labelTarget")}>{route.stackId} / {url.hostname}:{url.port || "80"}</Detail><Detail label={t("labelTls")}>{route.resolver}</Detail><Detail label={t("labelMiddleware")}><div className="flex flex-wrap justify-end gap-2">{route.middlewares.map((name, i) => <span key={name}>{i + 1}. <MiddlewareHelp name={name} /></span>)}</div></Detail></dl></TabsContent><TabsContent value="raw"><pre className="max-h-72 overflow-auto rounded-lg bg-code p-3 text-xs leading-relaxed">{stringify({ labels })}</pre></TabsContent></Tabs><p className="mt-3 text-xs text-muted-foreground">{t("labelsHint")}</p></section>;
}
