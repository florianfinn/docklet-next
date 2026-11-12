import { Award, FileKey2, RefreshCw, Trash2 } from "lucide-react";
import { Link } from "react-router";
import { useFormatter } from "use-intl";
import { Button } from "@/shared/ui/button";
import { ActionsMenu, type MenuGroups } from "@/shared/components/action-menu";
import { OverviewTable, OverviewRowView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import { isNestedInteractive } from "@/shared/lib/row-navigation";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { coversDomain, type ProxyCertificate, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
export function CertificateCollection({ certificates, routes, layout, selectedId, onEdit, onRenew, onRemove }: { certificates: ProxyCertificate[]; routes: ProxyRoute[]; layout: "manager" | "workspace"; selectedId: string | null; onEdit: (cert: ProxyCertificate) => void; onRenew: (cert: ProxyCertificate) => void; onRemove: (cert: ProxyCertificate) => void }) {
  const t = useProxyText();
  const format = useFormatter();
  function menu(cert: ProxyCertificate): MenuGroups {
    return [[{ id: "edit", label: t(cert.origin === "imported" ? "replace" : "edit"), icon: <FileKey2 />, onSelect: () => onEdit(cert) }, ...(cert.origin === "automatic" && cert.managed ? [{ id: "renew", label: t("renew"), icon: <RefreshCw />, onSelect: () => onRenew(cert) }] : [])],
      cert.origin === "imported" || cert.managed ? [{ id: "remove", label: t(cert.origin === "imported" ? "removeImport" : "stopAutomation"), icon: <Trash2 />, onSelect: () => onRemove(cert) }] : []];
  }
  function actions(cert: ProxyCertificate) { return <ActionsMenu groups={menu(cert)} label={t("moreActions") + " · " + cert.name} />; }
  const expiry = (cert: ProxyCertificate) => format.dateTime(new Date(cert.expires + "T12:00:00Z"), { dateStyle: "medium" });
  const linked = (cert: ProxyCertificate) => routes.filter((r) => r.proxy === cert.proxy && r.tls !== "off" && coversDomain(cert.domains, r.domain));
  const columns: OverviewColumn[] = [{ id: "name", label: t("domains"), className: "min-w-52 pl-3" }, ...(["issuer", "expires", "status", "usedBy"] as const).map((id) => ({ id, label: t(id) }))];
  if (layout === "manager") return <OverviewTable fitKey={certificates.map((c) => c.id).join(",")} columns={() => columns} dense={false} selecting={false} actionsLabel={t("actions")} listLabel={t("certificates")}>{(view, cols) => certificates.map((cert) => <OverviewRowView key={cert.id} view={view} columns={cols} pad="py-4" hostColor="var(--primary)" selected={selectedId === cert.id} menu={menu(cert)} menuTitle={cert.name} actions={actions(cert)}
    onClick={(e) => { if (e.defaultPrevented || e.button !== 0 || !e.currentTarget.contains(e.target as Node) || isNestedInteractive(e.target, e.currentTarget) || window.getSelection()?.toString()) return; onEdit(cert); }}
    cells={{
      name: <button onClick={() => onEdit(cert)} className="flex items-center gap-3 rounded-sm text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"><IconTile tone="violet"><Award /></IconTile><span><strong className="block text-sm">{cert.name}</strong><span className="mt-1 block font-mono text-xs text-muted-foreground">{cert.domains.join(", ")}</span></span></button>,
      issuer: <><span className="text-sm">{cert.issuer}</span><p className="mt-1 text-xs text-muted-foreground">{cert.resolver || t(cert.origin)}</p></>,
      expires: <span className="text-sm">{expiry(cert)}</span>,
      status: <><ToneBadge tone={cert.status === "expiresSoon" ? "warn" : "ok"}>{t(cert.status)}</ToneBadge>{cert.origin === "automatic" && !cert.managed && <p className="mt-1 text-xs text-warning">{t("automationStopped")}</p>}</>,
      usedBy: <span className="text-sm">{t("routeCount", { count: linked(cert).length })}</span>,
    }} />)}</OverviewTable>;
  return <div className="flex flex-col gap-4">{certificates.map((cert) => <article key={cert.id} className={"overflow-hidden rounded-xl border bg-card " + (selectedId === cert.id ? "ring-1 ring-primary" : "")}><div className="flex flex-wrap items-center gap-3 border-b p-4"><IconTile tone="ok"><Award /></IconTile><div className="min-w-0 flex-1"><h3 className="font-semibold">{cert.name}</h3><p className="mt-1 text-xs text-muted-foreground">{cert.issuer} · {t("expires")} {expiry(cert)}</p></div><ToneBadge tone={cert.status === "expiresSoon" ? "warn" : "ok"}>{t(cert.status)}</ToneBadge>{actions(cert)}</div><div className="grid gap-5 p-5 md:grid-cols-[1fr_1.2fr]"><section><h4 className="mb-3 text-xs text-muted-foreground">{t("certificateScope")}</h4><div className="flex flex-wrap gap-2">{cert.domains.map((domain) => <ToneBadge key={domain} tone={domain.startsWith("*.") ? "violet" : "cyan"}>{domain}</ToneBadge>)}</div>{cert.origin === "automatic" && !cert.managed && <p className="mt-3 text-xs text-warning">{t("automationStopped")}</p>}</section><section><h4 className="mb-3 text-xs text-muted-foreground">{t("usedRoutes")}</h4><div className="flex flex-wrap gap-2">{linked(cert).map((route) => <Button key={route.id} variant="outline" size="sm" asChild><Link to={"/proxy?instance=" + cert.proxy + "&route=" + route.id}>{route.domain}</Link></Button>)}{!linked(cert).length && <span className="text-sm text-muted-foreground">{t("noUsage")}</span>}</div></section></div></article>)}</div>;
}
