// Source: web/src/features/design-lab/proxy/ManagerRoutes.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Production Route DTOs and shared overview rows; real certificate check spans a companion row.

// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { Fragment, useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight, FileText, Globe, LockKeyhole, LockKeyholeOpen, Pencil, Trash2 } from "lucide-react";
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
import { Choice, EligibilityView } from "./controls";
import { CertificateCheck } from "./CertificateCheck";
import { canEdit } from "./model";
import { simpleHost } from "./suggestions";
import type { Instance, Route } from "./types";
import type { ChangeAction } from "./ChangeDialog";

export function RouteList({ instance, routes, hashes, writable, edit, action, create }: {
  instance: Instance; routes: Route[]; hashes?: Record<string, string>; writable: boolean;
  edit: (r: Route) => void; action: (a: ChangeAction) => void; create?: () => void;
}) {
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
    [ui("routes"), routes.length], [ui("withTls"), routes.filter(r => r.tls || r.resolver).length],
    [ui("labelRoutes"), routes.filter(r => r.class === "label-read-only").length], [ui("readOnly"), routes.filter(r => !canEdit(r)).length],
  ];
  return <div className="grid min-w-0 gap-4">
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">{stats.map(([label, count]) => <div key={label} className="rounded-xl border bg-card p-4"><p className="text-xs text-muted-foreground">{label}</p><strong className="mt-2 block text-2xl tabular-nums">{count}</strong></div>)}</div>
    <div className="flex min-w-0 flex-wrap items-end gap-3">
      <div className="min-w-[min(100%,16rem)] flex-1"><label htmlFor="route-search" className="mb-2 block text-sm font-medium">{ui("searchRoutes")}</label><Input id="route-search" value={search} onChange={e => setSearch(e.target.value)} placeholder={ui("searchRoutesHint")} /></div>
      <Choice id="route-source" label={ui("sourceFilter")} value={source} onChange={setSource} options={[{ value: "all", label: ui("allSources") }, { value: "file", label: ui("fileSource") }, { value: "labels", label: ui("dockerLabels") }]} />
      <p role="status" className="pb-2 text-sm text-muted-foreground">{ui("routeCount", { count: filtered.length, total: routes.length })}</p>
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
            name: <div className="flex min-w-0 items-start gap-3"><IconTile tone="cyan"><Globe /></IconTile><div className="min-w-0 [overflow-wrap:anywhere]"><h3 className="font-medium">{simpleHost(r.rule) ?? r.rule}</h3><p className="text-xs text-muted-foreground">{r.name}</p><p className="mt-1 font-mono text-xs text-muted-foreground">{r.file}</p><EligibilityView value={r.eligibility} /></div></div>,
            destination: <div className="min-w-0 space-y-1 text-xs [overflow-wrap:anywhere]"><span className="flex items-center gap-1"><ArrowUpRight className="size-3 shrink-0" />{r.service}</span>{r.upstreams?.map((u, n) => <p key={n} className="font-mono">{u}</p>)}{r.serviceFile && !r.upstreams?.length && <p>{ui("opaqueService", { name: r.serviceFile })}</p>}{r.class === "label-read-only" && <><p>{r.rule}</p><ul aria-label={ui("references")}>{(r.references ?? []).map((ref, n) => <li key={n}>{ref.name} · {t(PROXY_REFERENCE_KEYS[ref.kind])}</li>)}</ul>{r.projectId && <Link className="text-primary underline" to={`/stacks/${encodeURIComponent(inventoryId(instance.environmentId, r.projectId))}?tab=editor`}>{ui("stackEditor")}</Link>}{r.containerName && <p>{r.containerName}</p>}</>}</div>,
            tls: <span className="flex items-start gap-1 text-xs [overflow-wrap:anywhere]">{r.tls || r.resolver ? <LockKeyhole className="size-3 shrink-0 text-success" /> : <LockKeyholeOpen className="size-3 shrink-0 text-muted-foreground" />}{r.resolver || ui(r.tls ? "routeTls" : "noRouteCertificate")}</span>,
            source: <ToneBadge tone={r.class === "label-read-only" ? "accent" : "cyan"}><FileText className="size-3" />{ui(r.class === "label-read-only" ? "dockerLabels" : "fileSource")}</ToneBadge>,
            status: <ToneBadge tone={canEdit(r) ? "ok" : r.class === "infrastructure" ? "gray" : "warn"}>{ui(r.class === "infrastructure" ? "classes.infrastructure" : canEdit(r) ? "classes.editable" : "readOnly")}</ToneBadge>,
            certificate: cards ? certificate : null,
          }} />
          {!cards && certificate}
        </Fragment>;
      })}
    </OverviewView>
  </div>;
}
