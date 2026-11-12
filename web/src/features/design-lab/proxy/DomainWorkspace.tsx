// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { ArrowRight, Box, FileCode2, Globe, Layers, LockKeyhole, Pencil, SlidersHorizontal } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { RouteStatus } from "./controls";
import { LabelDetails } from "./LabelDetails";
import { MiddlewareHelp } from "./MiddlewareHelp";
import { RouteActions, type RouteActionsProps } from "./RouteActions";
import type { EditorStep } from "./RouteEditor";
import { matchingCertificates, useProxy, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
export function DomainWorkspace({ routes, selected, onSelect, actions, onEditStep }: { routes: ProxyRoute[]; selected?: ProxyRoute; onSelect: (id: string) => void; actions: (route: ProxyRoute) => RouteActionsProps; onEditStep: (route: ProxyRoute, step: EditorStep) => void }) {
  const t = useProxyText();
  const { certificates } = useProxy();
  const groups = [...new Set(routes.map((r) => r.domain.split(".").slice(-2).join(".")))].sort();
  const certs = selected ? matchingCertificates(selected, certificates) : [];
  const related = selected ? routes.filter((r) => r.id !== selected.id && r.url === selected.url) : [];
  return <div className="grid items-start gap-4 xl:grid-cols-[15rem_minmax(0,1fr)]">
    <nav aria-label={t("browseRoutes")} className="overflow-hidden rounded-xl border bg-card"><p className="border-b px-4 py-3 text-sm font-semibold">{t("domainsColumn")}</p><div className="max-h-[42rem] overflow-y-auto p-2">{groups.map((group) => <section key={group}><h3 className="px-3 pb-2 pt-4 text-xs font-semibold text-muted-foreground">{group}</h3>{routes.filter((r) => r.domain.endsWith(group)).map((route) => <button key={route.id} aria-current={route.id === selected?.id ? "true" : undefined} onClick={() => onSelect(route.id)} className="flex w-full items-center gap-3 rounded-lg border border-transparent px-3 py-3 text-left hover:bg-muted/40 aria-[current=true]:border-primary/40 aria-[current=true]:bg-primary/10"><Globe className="size-4 shrink-0 text-cyan" /><span className="min-w-0"><span className="block truncate text-sm font-medium" title={route.domain}>{route.domain}</span><span className="mt-1 block truncate font-mono text-[11px] text-muted-foreground">{route.url.replace(/^https?:\/\//, "")}</span></span><span className={"ml-auto size-1.5 shrink-0 rounded-full " + (route.enabled ? "bg-success" : "bg-muted-foreground")} /></button>)}</section>)}</div></nav>
    {selected && <Card className="min-w-0 gap-0 overflow-hidden py-0"><CardHeader className="flex flex-wrap items-center justify-between gap-4 border-b p-5"><div className="min-w-0"><CardTitle className="break-all text-xl">{selected.domain}</CardTitle><div className="mt-2"><RouteStatus enabled={selected.enabled} /></div></div><div className="flex items-center gap-2"><Button size="sm" onClick={actions(selected).onEdit}><Pencil data-icon="inline-start" />{t(selected.source === "file" ? "editRoute" : "openDetails")}</Button><RouteActions {...actions(selected)} /></div></CardHeader><CardContent className="p-5">
      <div className="grid items-stretch gap-2 lg:grid-cols-[1fr_auto_1fr_auto_1fr]">
        <button onClick={() => onEditStep(selected, "address")} className="flex min-w-0 flex-col items-start gap-3 rounded-xl border border-cyan/25 bg-cyan/5 p-4 text-left hover:border-cyan/60"><IconTile tone="cyan"><Globe /></IconTile><span className="text-xs text-muted-foreground">{t("domain")}</span><strong className="break-all text-sm">{selected.domain}</strong><code className="text-xs text-muted-foreground">{selected.path || "/"}</code><span className="mt-auto text-xs text-primary-text">{t("changeAddress")}</span></button>
        <ArrowRight className="size-4 self-center justify-self-center text-muted-foreground max-lg:rotate-90" />
        <button onClick={() => onEditStep(selected, "tls")} className="flex min-w-0 flex-col items-start gap-3 rounded-xl border border-success/25 bg-success/5 p-4 text-left hover:border-success/60"><IconTile tone="ok"><LockKeyhole /></IconTile><span className="text-xs text-muted-foreground">{t("sslSelection")}</span><strong className="break-all text-sm">{selected.tls === "off" ? t("noSsl") : certs[0]?.domains.join(", ") ?? t("needsCertificate")}</strong><span className="text-xs text-muted-foreground">{selected.tls === "auto" ? selected.resolver : certs[0]?.issuer}</span><span className="mt-auto text-xs text-primary-text">{t("changeSsl")}</span></button>
        <ArrowRight className="size-4 self-center justify-self-center text-muted-foreground max-lg:rotate-90" />
        <button onClick={() => onEditStep(selected, "destination")} className="flex min-w-0 flex-col items-start gap-3 rounded-xl border border-primary/25 bg-primary/5 p-4 text-left hover:border-primary/60"><IconTile tone="accent"><Box /></IconTile><span className="text-xs text-muted-foreground">{t("target")}</span><strong className="break-all font-mono text-sm">{selected.url.replace(/^https?:\/\//, "")}</strong><span className="text-xs text-muted-foreground">{selected.stackId ?? t("externalTarget")}</span><span className="mt-auto text-xs text-primary-text">{t("changeDestination")}</span></button>
      </div>
      <div className="mt-5 grid gap-5 rounded-xl border bg-muted/10 p-4 md:grid-cols-2"><div><h3 className="mb-3 flex items-center gap-2 text-sm font-medium"><Layers className="size-4 text-cyan" />{t("linkedServices")}</h3><div className="flex flex-wrap gap-2">{selected.containerId && <Button variant="outline" size="sm" asChild><Link to={"/containers/" + selected.containerId}>{t("openContainer")}</Link></Button>}{selected.stackId && <Button variant="outline" size="sm" asChild><Link to={"/stacks/" + selected.stackId + "?tab=proxy"}>{selected.stackId}</Link></Button>}{!selected.containerId && <code className="break-all text-xs text-muted-foreground">{selected.url}</code>}</div><h3 className="mb-2 mt-5 flex items-center gap-2 text-sm font-medium"><FileCode2 className="size-4 text-primary-text" />{t("configurationSource")}</h3><ToneBadge tone="gray">{selected.source === "file" ? t("fileName") : t("labels")}</ToneBadge></div><div><h3 className="mb-3 flex items-center gap-2 text-sm font-medium"><SlidersHorizontal className="size-4 text-primary-text" />{t("middlewares")}</h3><div className="flex flex-wrap gap-3 text-sm">{selected.middlewares.map((name, index) => <span key={name}>{index + 1}. <MiddlewareHelp name={name} /></span>)}{!selected.middlewares.length && <span className="text-muted-foreground">{t("noMiddleware")}</span>}</div></div></div>
      {selected.source === "labels" && <LabelDetails route={selected} />}
      <section className="mt-5 rounded-xl border p-4"><h3 className="text-sm font-medium">{t("relatedDestinations")}</h3>{related.length ? <div className="mt-3 flex flex-col gap-2">{related.map((r) => <button key={r.id} className="text-left text-sm text-primary-text hover:underline" onClick={() => onSelect(r.id)}>{r.domain}</button>)}</div> : <p className="mt-2 text-xs text-muted-foreground">{t("noRelatedRoutes")}</p>}</section>
    </CardContent></Card>}
  </div>;
}
