// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useProxyText } from "./use-proxy-text";
import { useState } from "react";
import { ArrowRight, Box, Copy, FileCode2, Globe, Layers, LockKeyhole, Network, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { Badge } from "@/shared/ui/badge";
import { IconTile } from "@/shared/components/tone";
import { Choice, Detail, RouteStatus } from "./controls";
import { duplicateRoute, routeName, matchingCertificates, newRoute, proxyStore, useProxy, type ProxyRoute } from "./model";
import { LabelDetails } from "./LabelDetails";
import { MiddlewareHelp } from "./MiddlewareHelp";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { DomainWorkspace } from "./DomainWorkspace";
import { ManagerRoutes } from "./ManagerRoutes";
import { useProxyDesign } from "./design";
import type { RouteActionsProps } from "./RouteActions";
import { RouteEditor, type EditorStep } from "./RouteEditor";

export function RoutePath({ route }: { route: ProxyRoute }) {
  const t = useProxyText();
  const { connections } = useProxy();
  const segments = [route.domain, connections[route.proxy]?.container ?? "Traefik", route.middlewares.join(" → ") || t("noMiddleware"), route.url.replace(/^https?:\/\//, "")];
  return <ol aria-label={t("routePath")} className="flex flex-wrap items-center gap-1.5 text-xs">{segments.map((part, i) => <li key={i} className="flex min-w-0 items-center gap-1.5">{i > 0 && <ArrowRight className="size-3 shrink-0 text-muted-foreground" />}<span className="rounded-md border bg-muted/30 px-2 py-1.5 break-all">{i === 2 && route.middlewares.length ? route.middlewares.map((name, index) => <span key={name}>{index > 0 && " → "}<MiddlewareHelp name={name} /></span>) : part}</span></li>)}</ol>;
}
export function RouteDetails({ route, onEdit, onDuplicate, onRemove }: { route: ProxyRoute; onEdit: () => void; onDuplicate: () => void; onRemove: () => void }) {
  const t = useProxyText();
  const { certificates } = useProxy();
  const certs = matchingCertificates(route, certificates);
  return <Card className="min-w-0 gap-0 overflow-hidden py-0">
    <CardHeader className="gap-3 border-b p-5"><div className="flex items-start gap-3"><IconTile tone="accent"><Globe /></IconTile><div className="min-w-0 flex-1"><CardTitle className="break-all text-base">{route.domain}</CardTitle><CardDescription className="mt-1">{route.path || "/"}</CardDescription></div></div><div><RouteStatus enabled={route.enabled} /></div></CardHeader>
    <CardContent className="p-5"><dl>
      <Detail label={t("target")}><span className="font-mono text-xs">{route.url}</span>{!!route.backends.length && <p className="text-xs text-muted-foreground">{t("loadBalance", { count: 1 + route.backends.length })}</p>}</Detail>
      <Detail label={t("tls")}>{t(route.tls === "off" ? "tlsOff" : route.tls === "auto" ? "tlsAuto" : "tlsCustom")}</Detail>
      {route.tls !== "off" && <Detail label={t("matchingCert")}><div className="flex flex-col items-end gap-1">{certs.length ? certs.map((c) => <Link key={c.id} to={"/proxy?tab=certificates&certificate=" + c.id + "&instance=" + route.proxy} className="text-primary-text hover:underline">{c.name}</Link>) : <span className="text-warning">{t("noCert")}</span>}</div></Detail>}
      <Detail label={t("source")}><span className="flex items-center gap-1.5"><FileCode2 className="size-3.5" />{route.source === "file" ? t("fileName") : t("labels")}</span></Detail>
    </dl>
    <div className="my-4 flex flex-wrap gap-2">
      {route.stackId && <Button size="sm" variant="outline" asChild><Link to={"/stacks/" + route.stackId + "?tab=proxy"}><Layers data-icon="inline-start" />{route.stackId}</Link></Button>}
      {route.containerId && <Button size="sm" variant="outline" asChild><Link to={"/containers/" + route.containerId}><Box data-icon="inline-start" />{t("openContainer")}</Link></Button>}
      {route.containerId && <Button size="sm" variant="outline" asChild><Link to={"/networks?host=" + route.hostId + "&node=" + route.containerId + "&proxyLayout=augmented"}><Network data-icon="inline-start" />{t("openNetwork")}</Link></Button>}
    </div>
    <p className="mb-2 text-xs font-medium text-muted-foreground">{t("routePath")}</p><RoutePath route={route} />
    {route.source === "labels" && <LabelDetails route={route} />}
    <div className="mt-6 flex flex-wrap gap-2">{route.source === "labels" ? <Button asChild size="sm"><Link to={"/stacks/" + route.stackId + "?tab=editor"}><FileCode2 data-icon="inline-start" />{t("openCompose")}</Link></Button> : <Button size="sm" onClick={onEdit}><Pencil data-icon="inline-start" />{t("edit")}</Button>}
      <Button size="sm" variant="outline" onClick={onDuplicate}><Copy data-icon="inline-start" />{t("duplicate")}</Button>
      {route.source === "file" && <Button size="icon-sm" variant="ghost" aria-label={t("remove")} onClick={onRemove}><Trash2 /></Button>}
    </div>
    </CardContent>
  </Card>;
}
export function RoutesView({ instance }: { instance: string }) {
  const t = useProxyText();
  const { routes } = useProxy();
  const design = useProxyDesign();
  const [inspecting, setInspecting] = useState<ProxyRoute | null>(null);
  const [status, setStatus] = useState("all");
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState("");
  const [source, setSource] = useState("all");
  const [editor, setEditor] = useState<{ route: ProxyRoute; creating: boolean; step?: EditorStep } | null>(null);
  const scope = params.get("container") ?? params.get("stack");
  const list = routes.filter((r) => r.proxy === instance && (status === "all" || (status === "ssl" ? r.tls !== "off" : !r.enabled)) && (source === "all" || r.source === source) && (!scope || r.containerId === scope || r.stackId === scope) && (routeName(r) + " " + r.domain + " " + r.url + " " + r.stackId).toLowerCase().includes(query.toLowerCase()));
  const selected = list.find((r) => r.id === params.get("route")) ?? list[0];
  function select(id: string) { setParams((p) => { p.set("route", id); return p; }, { replace: true }); }
  function edit(route: ProxyRoute, step?: EditorStep) {
    select(route.id);
    if (route.source === "labels") { setInspecting(route); return; }
    setInspecting(null); setEditor({ route, creating: false, step });
  }
  function actions(route: ProxyRoute): RouteActionsProps {
    return { route, onEdit: () => edit(route), onInspect: () => setInspecting(route),
      onDuplicate: () => { setInspecting(null); setEditor({ route: duplicateRoute(route, routes), creating: true }); },
      onRemove: () => { setInspecting(null); proxyStore.set((s) => ({ ...s, routes: s.routes.filter((r) => r.id !== route.id) })); toast.success(t("removed")); },
    };
  }
  const totals = routes.filter((r) => r.proxy === instance);
  return <>
    {design === "manager" && <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">{([
      { key: "proxyHosts", count: totals.length, status: "all", source: "all" },
      { key: "secured", count: totals.filter((r) => r.tls !== "off").length, status: "ssl", source: "all" },
      { key: "labels", count: totals.filter((r) => r.source === "labels").length, status: "all", source: "labels" },
      { key: "disabled", count: totals.filter((r) => !r.enabled).length, status: "disabled", source: "all" },
    ] as const).map((item) => <button key={item.key} onClick={() => { setStatus(item.status); setSource(item.source); }} className="flex items-center gap-3 rounded-xl border bg-card p-4 text-left hover:border-primary/50"><IconTile tone={item.key === "secured" ? "ok" : "accent"}><Globe /></IconTile><span><strong className="block text-2xl">{item.count}</strong><span className="text-xs text-muted-foreground">{t(item.key)}</span></span></button>)}</div>}
    <div className="mb-4 flex flex-wrap items-center gap-3">
      <InputGroup className="w-full sm:max-w-sm"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput type="search" aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} /></InputGroup>
      <Choice label={t("source")} value={source} onChange={setSource} options={[{ value: "all", label: t("allSources") }, { value: "file", label: t("file") }, { value: "labels", label: t("labels") }]} />
      <span className="flex-1 text-xs text-muted-foreground">{t("routeCount", { count: list.length })}</span>
      <Button onClick={() => setEditor({ route: newRoute(instance), creating: true })}><Plus data-icon="inline-start" />{t("addRoute")}</Button>
    </div>
    {scope && <Button variant="ghost" size="sm" className="mb-3" onClick={() => setParams((p) => { p.delete("container"); p.delete("stack"); return p; })}>{t("clearFilter")} · {scope}</Button>}
    {status !== "all" && <Button className="mb-3" size="sm" variant="ghost" onClick={() => setStatus("all")}>{t("clearFilter")} · {t(status === "ssl" ? "secured" : "disabled")}</Button>}
    {design === "workspace" ? <DomainWorkspace routes={list} selected={selected} onSelect={select} actions={actions} onEditStep={edit} /> : design === "manager" ? <ManagerRoutes routes={list} actions={actions} /> : <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.55fr)_minmax(20rem,1fr)]">
      <div className="min-w-0 overflow-hidden rounded-xl border bg-card">
        <Table><TableHeader><TableRow>{(["domain", "target", "tls", "source"] as const).map((key) => <TableHead key={key}>{t(key)}</TableHead>)}</TableRow></TableHeader><TableBody>
          {list.map((r) => <TableRow key={r.id} data-state={selected?.id === r.id ? "selected" : undefined} className="h-19 cursor-pointer" onClick={() => select(r.id)}>
            <TableCell><button onClick={() => select(r.id)} className="flex items-center gap-3 text-left font-medium outline-offset-4"><IconTile size="sm" tone={r.enabled ? "cyan" : "gray"}><Globe /></IconTile><span>{r.domain}<span className="mt-1 block text-xs font-normal text-muted-foreground">{r.path || (r.enabled ? t("active") : t("disabled"))}</span></span></button></TableCell>
            <TableCell><code className="text-xs">{r.url.replace(/^https?:\/\//, "")}</code>{r.stackId && <p className="mt-1 text-xs text-muted-foreground">{r.stackId}</p>}</TableCell>
            <TableCell>{r.tls === "off" ? <span className="text-xs text-muted-foreground">{t("off")}</span> : <LockKeyhole className="size-4 text-success" aria-label={t("tls")} />}</TableCell>
            <TableCell><Badge variant="outline" className="text-xs">{t(r.source)}</Badge></TableCell>
          </TableRow>)}
        </TableBody></Table>
        {!list.length && <p className="p-10 text-center text-sm text-muted-foreground">{t("empty")}</p>}
      </div>
      {selected ? <RouteDetails {...actions(selected)} /> : <p className="p-8 text-center text-sm text-muted-foreground">{t("selectRoute")}</p>}
    </div>}
    {design !== "classic" && !list.length && <p className="p-10 text-center text-sm text-muted-foreground">{t("empty")}</p>}
    {inspecting && <Dialog open onOpenChange={(open) => !open && setInspecting(null)}><DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl"><DialogHeader><DialogTitle>{t("details")}</DialogTitle><DialogDescription>{inspecting.domain}</DialogDescription></DialogHeader><RouteDetails {...actions(inspecting)} /></DialogContent></Dialog>}
    {editor && <RouteEditor key={editor.route.id} initial={editor.route} creating={editor.creating} initialStep={editor.step} onClose={() => { if (proxyStore.get().routes.some((r) => r.id === editor.route.id)) select(editor.route.id); setEditor(null); }} />}
  </>;
}
