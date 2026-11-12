// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { ExternalLink, RotateCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Button } from "@/shared/ui/button";
import { ToneBadge } from "@/shared/components/tone";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { Detail } from "./controls";
import { discoverLab, labResultFailed, labRouteStatus, listLabRoutes, reapplyLab, removeLabRoutes, useLab, type LabResult } from "./lab-bridge";
import type { ProxyKind } from "./model";
import { useProxy, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
const hasDetails = (value: unknown) => !!value && (typeof value !== "object" || Object.keys(value).length > 0);
const display = (value: unknown) => typeof value === "string" ? value : value ? JSON.stringify(value) : "—";
export function LabOutcome({ result }: { result: LabResult }) {
  const t = useProxyText();
  return <section aria-label={t("labOutcome")} aria-live="polite" className="mt-4 min-w-0 rounded-xl border bg-muted/20 p-4 text-sm">
    <ToneBadge tone={labResultFailed(result) ? "err" : "ok"}>{t(labResultFailed(result) ? "labError" : "labAccepted")}</ToneBadge>
    {result.error && <p role="alert" className="mt-2 whitespace-pre-wrap break-words text-destructive">{result.stage} · {result.error}</p>}
    {hasDetails(result.warnings) && <p className="mt-2 break-all text-warning">{t("labWarnings")}: {display(result.warnings)}</p>}
    {hasDetails(result.errors) && <pre className="mt-2 whitespace-pre-wrap break-all text-xs text-destructive">{display(result.errors)}</pre>}
    {!!(result.router || result.service) && <details className="mt-2"><summary className="cursor-pointer text-xs text-muted-foreground">{t("labRouterService")}</summary><pre className="mt-2 whitespace-pre-wrap break-all text-xs">{display({ router: result.router, service: result.service })}</pre></details>}
    {!!result.probe?.length && <div className="mt-3 overflow-x-auto"><Table><TableHeader><TableRow>{(["status", "labBackend", "issuer", "labRedirect"] as const).map((key) => <TableHead key={key}>{t(key)}</TableHead>)}</TableRow></TableHeader><TableBody>{result.probe.map((p, i) => <TableRow key={i}><TableCell>{p.error ? <span className="text-destructive">{p.error}</span> : p.status}</TableCell><TableCell>{p.backend || "—"}</TableCell><TableCell className="max-w-64 break-words whitespace-normal text-xs">{display(p.certificate?.issuer)}</TableCell><TableCell className="max-w-64 break-all whitespace-normal text-xs">{p.location || "—"}</TableCell></TableRow>)}</TableBody></Table></div>}
  </section>;
}
export function LabRouteStatus({ route }: { route: ProxyRoute }) {
  const t = useProxyText();
  const { connections } = useProxy();
  const { info, records } = useLab();
  const kind = connections[route.proxy]?.lab;
  if (!kind || !info) return null;
  const status = labRouteStatus(route, records[route.id]);
  const open = info.proxies.find((p) => p.kind === kind)?.hostnames.includes(route.domain);
  return <span className="mt-1 flex flex-wrap items-center gap-2"><ToneBadge tone={status === "labApplied" ? "ok" : status === "labError" ? "err" : "gray"}>{t(status)}</ToneBadge>{open && <a href={(route.tls === "off" ? "http://" : "https://") + route.domain + (route.path || "/")} target="_blank" rel="noopener noreferrer" aria-label={t("routeLink")} className="inline-flex items-center gap-1 text-xs text-primary-text hover:underline">{t("routeLink")}<ExternalLink className="size-3" /></a>}</span>;
}
function LabCleanup({ kind, instance }: { kind: ProxyKind; instance: string }) {
  const t = useProxyText();
  const { routes } = useProxy();
  const [names, setNames] = useState<string[] | null>(null);
  const tracked = new Set(routes.filter((r) => r.proxy === instance).flatMap((r) => r.labNames ?? []));
  const open = async () => {
    try {
      const live = await listLabRoutes(kind);
      if (live.length) setNames(live); else toast.info(t("labCleanupEmpty"));
    } catch { toast.error(t("labCleanupListFailed")); }
  };
  return <>
    <Button size="sm" variant="outline" onClick={() => void open()}><Trash2 data-icon="inline-start" />{t("labCleanup")}</Button>
    {names && <ConfirmDialog open onOpenChange={(next) => { if (!next) setNames(null); }} destructive title={t("labCleanupTitle")} description={t("labCleanupBody", { count: names.length })} confirmLabel={t("labCleanupConfirm")} onConfirm={async () => {
      const result = await removeLabRoutes(kind, names);
      if (!result.ok) throw new Error(result.error ?? result.stage ?? "lab-cleanup");
      toast.success(t("labCleanupDone", { count: names.length }));
    }}>
      <ul className="max-h-48 overflow-y-auto text-sm">{names.map((name) => <li key={name} className="flex items-center gap-2 break-all py-0.5">{name}{tracked.has(name) && <ToneBadge tone="gray">{t("labCleanupThisInstance")}</ToneBadge>}</li>)}</ul>
    </ConfirmDialog>}
  </>;
}
export function LabSettings({ instance }: { instance: string }) {
  const t = useProxyText();
  const { connections, routes } = useProxy();
  const { info, records, busy } = useLab();
  const proxy = info?.proxies.find((p) => p.kind === connections[instance]?.lab);
  if (!proxy) return null;
  return <section className="mt-5 rounded-xl border bg-card p-5"><div className="flex flex-wrap items-center gap-3"><h3 className="flex-1 font-semibold">{t("labTitle")}</h3><Button size="sm" variant="outline" onClick={() => void discoverLab()}>{t("labRefresh")}</Button><LabCleanup kind={proxy.kind} instance={instance} /><Button size="sm" disabled={busy} onClick={() => void reapplyLab(instance)}><RotateCw data-icon="inline-start" />{t(busy ? "labApplying" : "labReapply")}</Button></div>
    <dl className="mt-3"><Detail label={t("status")}>{t(proxy.reachable ? "labReachable" : "labUnreachable")}</Detail><Detail label={t("labVersion")}>{proxy.version ?? proxy.image}</Detail><Detail label={t("labReload")}>{proxy.reload}</Detail><Detail label={t("labCapabilities")}><span className="flex max-w-xl flex-wrap justify-end gap-2">{(Object.entries(proxy.capabilities) as [keyof typeof proxy.capabilities, boolean][]).map(([key, enabled]) => <ToneBadge key={key} tone={enabled ? "ok" : "gray"}>{t(({ dnsChallenge: enabled ? "labDns" : "labNoDns", labels: enabled ? "labLabels" : "labNoLabels", publicAcme: enabled ? "labPublicAcme" : "labNoPublicAcme", localCa: enabled ? "labLocalCa" : "labNoLocalCa" } as const)[key])}</ToneBadge>)}</span></Detail><Detail label={t("labHostnames")}>{proxy.hostnames.join(", ")}</Detail></dl>
    <p className="mt-3 text-xs text-muted-foreground">{t("labHint")}</p>
    {routes.filter((r) => r.proxy === instance).map((route) => <div key={route.id} className="mt-3 border-t pt-3"><strong className="break-all text-sm">{route.domain}</strong><LabRouteStatus route={route} />{records[route.id] && <LabOutcome result={records[route.id]!.result} />}</div>)}
  </section>;
}
