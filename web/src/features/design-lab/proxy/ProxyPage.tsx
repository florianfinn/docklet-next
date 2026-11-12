// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useProxyText } from "./use-proxy-text";
import { Columns3, Globe, List, RotateCcw } from "lucide-react";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { PageHeader } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Tabs, TabsContent } from "@/shared/ui/tabs";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { ProxyTabs } from "./ProxyTabs";
import { ProxyInstances } from "./ProxyInstances";
import { useLab } from "./lab-bridge";
import { proxyStore, useProxy } from "./model";
import { designStore, useProxyDesign, type ProxyDesign } from "./design";
import { RoutesView } from "./RoutesView";
import { CertificatesView } from "./CertificatesView";
import { ProxySettings } from "./ProxySettings";
import { ProxySetup } from "./ProxySetup";
import { RouteGraph } from "./RouteGraph";
import { MapIdeas } from "./MapIdeas";
const TABS = ["routes", "certificates", "maps", "settings"] as const;
const DESIGNS = [
  { id: "classic", labelKey: "classicDesign", hint: "classicHint", icon: Columns3 },
  { id: "manager", labelKey: "managerDesign", hint: "managerHint", icon: List },
  { id: "workspace", labelKey: "workspaceDesign", hint: "workspaceHint", icon: Globe },
] as const;
export default function ProxyPage() {
  const t = useProxyText();
  const state = useProxy();
  const { info } = useLab();
  const design = useProxyDesign();
  const [params, setParams] = useSearchParams();
  const instance = state.instances.find((p) => p.id === params.get("instance"))?.id ?? state.instance;
  const requested = params.get("tab");
  const tab = requested === "setup" || requested === "configuration" ? "settings" : TABS.find((tab) => tab === requested) ?? "routes";
  const configured = !!state.connections[instance];
  function navigate(value: string) { setParams((p) => { p.set("tab", value); return p; }); }
  function selectInstance(next: string, tab?: string) {
    proxyStore.set((s) => ({ ...s, instance: next }));
    setParams((p) => { p.set("instance", next); if (tab) p.set("tab", tab); for (const key of ["route", "container", "stack", "cert"]) p.delete(key); return p; });
  }
  return <>
    <PageHeader title={t("title")} description={t("subtitle")} actions={<SimulationBadge />} />
    <ProxyInstances instance={instance} onSelect={selectInstance} onSettings={() => navigate("settings")} />
    <details className="mb-4"><summary className="mb-3 cursor-pointer text-xs text-muted-foreground">{t("compareDesigns")}</summary>
    <RadioGroup aria-label={t("designLabel")} value={design} onValueChange={(value) => designStore.set(value as ProxyDesign)} className="mb-4 grid gap-3 md:grid-cols-3">{DESIGNS.map((item) => <label key={item.id} htmlFor={"proxy-design-" + item.id} className="flex cursor-pointer items-center gap-3 rounded-xl border bg-card p-4 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary/10"><RadioGroupItem id={"proxy-design-" + item.id} value={item.id} className="sr-only" /><item.icon className="size-5 shrink-0 text-primary-text" /><span><span className="block text-sm font-semibold">{t(item.labelKey)}</span><span className="mt-1 block text-xs text-muted-foreground">{t(item.hint)}</span></span></label>)}</RadioGroup></details>
    <div className="mb-5 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>{t(info ? "labSimulation" : "simulation")}</span><Button variant="ghost" size="sm" onClick={() => { proxyStore.reset(); toast.success(t("resetDone")); }}><RotateCcw data-icon="inline-start" />{t("reset")}</Button></div>
    {!configured ? <ProxySetup key={instance} instance={instance} onConfigured={(id) => selectInstance(id, "routes")} /> : <Tabs value={tab} onValueChange={navigate}>
      <ProxyTabs options={TABS.map((value) => ({ value, label: t(value === "routes" && design === "manager" ? "proxyHosts" : value) }))} />
      <TabsContent value="routes"><RoutesView key={instance + design} instance={instance} /></TabsContent>
      <TabsContent value="certificates"><CertificatesView key={instance + design} instance={instance} /></TabsContent>
      <TabsContent value="settings"><ProxySettings key={instance} instance={instance} /></TabsContent>
      <TabsContent value="maps"><RouteGraph routes={state.routes.filter((r) => r.proxy === instance)} /><details className="mt-5 rounded-xl border p-4"><summary className="cursor-pointer text-sm font-medium">{t("compareIdeas")}</summary><div className="mt-4"><MapIdeas showGraph={false} instance={instance} routes={state.routes.filter((r) => r.proxy === instance)} /></div></details></TabsContent>
    </Tabs>}
  </>;
}
