// Source: web/src/features/design-lab/proxy/ManagerRoutes.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Production Route DTOs and shared overview rows; real certificate check spans a companion row. Tile icons, compact toolbar, entry-point TLS, label summary (lab LabelDetails). M37 #581: left-out Unraid containers not named.

// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { Fragment, useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight, Boxes, Eye, FileText, Globe, LockKeyhole, LockKeyholeOpen, Pencil, Search, Server, Trash2 } from "lucide-react";
import { useTranslations } from "use-intl";
import { inventoryId } from "@/data/http/inventory-mapping";
import { useStacks } from "@/data/queries";
import { useMediaQuery } from "@/shared/hooks/use-media-query";
import { ActionsMenu, type MenuGroups } from "@/shared/components/action-menu";
import { OverviewRowView, OverviewView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { TableCell, TableRow } from "@/shared/ui/table";
import { PROXY_REFERENCE_KEYS } from "@/data/wire-catalogue-proxy";
import { Choice, Reason } from "./controls";
import { Detail } from "./presentation";
import { CertificateCheck } from "./CertificateCheck";
import { canEdit } from "./model";
import { simpleHost } from "./suggestions";
import type { Instance, Inventory, Route } from "./types";
import type { ChangeAction } from "./ChangeDialog";
import { useHiddenUnraidIds } from "@/shared/hooks/use-host-scope";

export function RouteList({ instance, routes, hashes, writable, edit, action, create, entryPoints = [] }: {
  instance: Instance; routes: Route[]; hashes?: Record<string, string>; writable: boolean; entryPoints?: Inventory["entryPoints"];
  edit: (r: Route) => void; action: (a: ChangeAction) => void; create?: () => void;
}) {
  // A label route names the container that serves it ("docker:<container>:<router>"); a left-out Unraid container is not named.
  const hiddenIds = useHiddenUnraidIds();
  const labelContainerHidden = (r: Route) => {
    const raw = r.id.startsWith("docker:") ? r.id.split(":")[1] : undefined;
    return !!raw && (hiddenIds.has(`${instance.environmentId}~${raw}`) || hiddenIds.has(raw));
  };
  const t = useTranslations(), ui = useTranslations("proxyContract.ui"), stacks = useStacks();
  const [search, setSearch] = useState(""), [source, setSource] = useState("all");
  const cards = useMediaQuery("(max-width: 767px)");
  const columns: OverviewColumn[] = [
    { id: "name", label: ui("domain"), className: "w-[30%]" },
    { id: "destination", label: ui("destination") },
    { id: "tls", label: ui("tlsGroup") },
    { id: "source", label: ui("source") },
    { id: "status", label: ui("status") },
    ...(cards ? [{ id: "certificate", label: ui("servedCertificate"), wide: true }] : []),
  ];
  const filtered = routes.filter(r => {
    const labels = r.class === "label-read-only";
    const stack = stacks.data?.find(s => r.projectId && s.id === inventoryId(instance.environmentId, r.projectId));
    return (source === "all" || (source === "labels") === labels) && [r.name, r.rule, r.service, r.containerName, stack?.name, ...(r.upstreams ?? [])].join(" ").toLocaleLowerCase().includes(search.toLocaleLowerCase());
  });
  const stats = [
    { label: ui("routes"), count: routes.length, icon: <Globe />, tone: "cyan" },
    { label: ui("withTls"), count: routes.filter(r => r.tls || r.resolver).length, icon: <LockKeyhole />, tone: "ok" },
    { label: ui("labelRoutes"), count: routes.filter(r => r.class === "label-read-only").length, icon: <Boxes />, tone: "accent" },
    { label: ui("readOnly"), count: routes.filter(r => !canEdit(r)).length, icon: <Eye />, tone: "warn" },
  ] as const;
  // Route TLS, or TLS that an explicit or default entry point provides (break-map row 2).
  const tlsOf = (r: Route) => {
    if (r.tls || r.resolver) return { on: true, text: r.resolver || ui("routeTls") };
    const selected = r.entryPoints?.length ? (entryPoints ?? []).filter(ep => r.entryPoints!.includes(ep.name)) : [];
    if (selected.some(ep => ep.tls)) return { on: true, text: ui("tlsFromEntryPoint") };
    if (!r.entryPoints?.length && (entryPoints ?? []).some(ep => ep.tls)) return { on: false, text: ui("tlsMayFromDefaults") };
    return { on: false, text: ui("noRouteCertificate") };
  };
  return <div className="grid min-w-0 gap-4">
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">{stats.map(s => <div key={s.label} className="flex min-w-0 items-center gap-3 rounded-xl border bg-card p-4"><IconTile tone={s.tone}>{s.icon}</IconTile><div className="min-w-0"><strong className="block text-2xl leading-none tabular-nums">{s.count}</strong><p className="mt-1 text-xs text-muted-foreground">{s.label}</p></div></div>)}</div>
    <div className="flex min-w-0 flex-wrap items-center gap-3">
      <div className="relative min-w-[min(100%,16rem)] flex-1"><label htmlFor="route-search" className="sr-only">{ui("searchRoutes")}</label><Search aria-hidden className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input id="route-search" className="pl-9" value={search} onChange={e => setSearch(e.target.value)} placeholder={ui("searchRoutesHint")} /></div>
      <div className="w-full sm:w-48"><Choice id="route-source" hideLabel label={ui("sourceFilter")} value={source} onChange={setSource} options={[{ value: "all", label: ui("allSources") }, { value: "file", label: ui("fileSource") }, { value: "labels", label: ui("dockerLabels") }]} /></div>
      <p role="status" className="text-sm text-muted-foreground">{ui("routeCount", { count: filtered.length, total: routes.length })}</p>
      {create && <Button disabled={!writable} onClick={create}>{ui("addRoute")}</Button>}
    </div>
    {filtered.length === 0 && <p className="p-5 text-sm">{ui("empty")}</p>}
    <OverviewView view={cards ? "cards" : "table"} columns={columns} dense={false} selecting={false} actionsLabel={ui("actionsHeader")} listLabel={ui("routes")}>
      {filtered.map(r => {
        const menu: MenuGroups = canEdit(r) && writable ? [[
          { id: "edit", label: ui("edit"), icon: <Pencil />, onSelect: () => edit(r) },
          ...(instance.kind === "traefik" && (r.dependants?.length ?? 0) > 1 && !!r.upstreams?.length ? [{ id: "service", label: ui("editService"), icon: <Pencil />, onSelect: () => action({ kind: "edit-service", target: r.file, route: r }) }] : []),
          { id: "delete", label: ui("delete"), icon: <Trash2 />, onSelect: () => action({ kind: "delete-route", target: r.file, route: r }) },
        ]] : [];
        const certificate = hashes?.[r.file] ? <CertificateCheck instance={instance} route={r} hash={hashes[r.file]!} frame={cards ? undefined : content => <TableRow data-certificate-row className="border-0 hover:bg-transparent"><TableCell colSpan={columns.length + 1} className="min-w-0 whitespace-normal px-4 pb-4 pt-0">{content}</TableCell></TableRow>} /> : null;
        return <Fragment key={r.id}>
          <OverviewRowView view={cards ? "cards" : "table"} columns={columns} pad="px-3 py-4" menu={menu.length ? menu : undefined} menuTitle={r.name} actions={menu.length ? <ActionsMenu groups={menu} label={ui("actions", { name: r.name })} /> : null} cells={{
            name: <div className="flex min-w-0 items-start gap-3"><IconTile tone="cyan"><Globe /></IconTile><div className="min-w-0 [overflow-wrap:anywhere]"><h3 className="font-medium">{simpleHost(r.rule) ?? r.rule}</h3><p className="text-xs text-muted-foreground">{r.name}</p><p className="mt-1 font-mono text-xs text-muted-foreground">{r.file}</p></div></div>,
            destination: r.class === "label-read-only" ? <LabelSummary route={r} hidden={labelContainerHidden(r)} tls={tlsOf(r).text} stackHref={r.projectId ? `/stacks/${encodeURIComponent(inventoryId(instance.environmentId, r.projectId))}?tab=editor` : undefined} kind={ref => t(PROXY_REFERENCE_KEYS[ref])} /> : <div className="min-w-0 space-y-1 text-xs [overflow-wrap:anywhere]"><span className="flex items-center gap-1"><ArrowUpRight className="size-3 shrink-0" />{r.service}</span>{r.upstreams?.map((u, n) => <p key={n} className="font-mono">{u}</p>)}{r.serviceFile && !r.upstreams?.length && <p>{ui("opaqueService", { name: r.serviceFile })}</p>}</div>,
            tls: (() => { const tls = tlsOf(r); return <span className="flex items-start gap-1 text-xs [overflow-wrap:anywhere]">{tls.on ? <LockKeyhole className="size-3 shrink-0 text-success" /> : <LockKeyholeOpen className="size-3 shrink-0 text-muted-foreground" />}{tls.text}</span>; })(),
            source: r.class === "infrastructure" ? <ToneBadge tone="gray"><Server className="size-3" />{ui("internalSource")}</ToneBadge> : <ToneBadge tone={r.class === "label-read-only" ? "accent" : "cyan"}>{r.class === "label-read-only" ? <Boxes className="size-3" /> : <FileText className="size-3" />}{ui(r.class === "label-read-only" ? "dockerLabels" : "fileSource")}</ToneBadge>,
            status: <div className="grid justify-items-start gap-1"><ToneBadge tone={canEdit(r) ? "ok" : r.class === "infrastructure" ? "gray" : "warn"}>{ui(r.class === "infrastructure" ? "classes.infrastructure" : canEdit(r) ? "classes.editable" : "readOnly")}</ToneBadge>{!canEdit(r) && r.class !== "infrastructure" && r.eligibility.reason && <span className="text-xs text-muted-foreground [overflow-wrap:anywhere]"><Reason code={r.eligibility.reason} /></span>}</div>,
            certificate: cards ? certificate : null,
          }} />
          {!cards && certificate}
        </Fragment>;
      })}
    </OverviewView>
  </div>;
}

/** Readable label-route summary as in the lab's LabelDetails, from the agent's Route fields. */
function LabelSummary({ route, tls, stackHref, kind, hidden }: { route: Route; tls: string; stackHref?: string; kind: (k: NonNullable<Route["references"]>[number]["kind"]) => string; hidden: boolean }) {
  const ui = useTranslations("proxyContract.ui"), th = useTranslations("common.hidden");
  return <div className="min-w-0 text-xs">
    <dl className="grid min-w-0">
      <Detail label={ui("labelMatch")}><span className="font-mono">{route.rule}</span></Detail>
      <Detail label={ui("labelTarget")}>{[route.service, hidden ? th("count", { count: 1 }) : route.containerName].filter(Boolean).join(" · ")}</Detail>
      <Detail label={ui("labelTls")}>{tls}</Detail>
      <Detail label={ui("labelMiddlewares")}>{route.references?.length ? <ul aria-label={ui("references")}>{route.references.map((ref, n) => <li key={n}>{ref.name} · {kind(ref.kind)}</li>)}</ul> : ui("none")}</Detail>
    </dl>
    {stackHref && <Link className="mt-1 inline-block text-primary underline" to={stackHref}>{ui("stackEditor")}</Link>}
  </div>;
}
