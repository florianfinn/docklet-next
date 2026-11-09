// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original shared plain-text advisory content and exact occurrence actions. M21 #437: advisory detail.

import { ExternalLink } from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router";
import { useFormatter, useTranslations } from "use-intl";
import type { Host } from "@/domain/model";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { AdvisoryOccurrence, AdvisoryRead } from "@/data/vulnerability-advisory";
import { QueryError } from "@/shared/components/page";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Skeleton } from "@/shared/ui/skeleton";

import { occurrenceSelectionKey, selectionFor } from "./advisory-selection";

function safeReference(value: string) {
  try { const url = new URL(value); return ["http:", "https:"].includes(url.protocol) ? url.href : undefined; } catch { return undefined; }
}

function Occurrence({ hostId, occurrence: o, selected, usageAvailable = true }: { hostId: string; occurrence: AdvisoryOccurrence; selected: boolean; usageAvailable?: boolean }) {
  const t = useTranslations("security"), format = useFormatter(), adapter = useAdapter();
  const capability = useCapability(o.ignored ? "unignore-vulnerability" : "ignore-vulnerability", { kind: "host", id: hostId });
  const [reason, setReason] = useState(""), [busy, setBusy] = useState(false), [error, setError] = useState(false);
  const reasonId = useId();
  const act = async () => {
    setBusy(true); setError(false);
    try { if (o.ignored && o.ignoreId) await adapter.unignoreVulnerability(hostId, o.ignoreId); else await adapter.ignoreVulnerability(hostId, o.vulnerabilityId, o, reason); }
    catch { setError(true); } finally { setBusy(false); }
  };
  return <Card data-occurrence data-selected={selected || undefined} className="min-w-0 data-[selected]:outline data-[selected]:outline-primary">
    <CardHeader><CardTitle className="flex flex-wrap items-center gap-2"><span>{o.pkgName}</span><Badge variant="outline">{t(`severities.${o.severity.toLowerCase() as "high"}`)}</Badge>{o.ignored && <Badge variant="secondary">{t("detail.ignored")}</Badge>}</CardTitle></CardHeader>
    <CardContent className="flex min-w-0 flex-col gap-3 [overflow-wrap:anywhere]">
      <p className="font-mono text-xs">{o.imageRefs.join(", ") || o.imageId}</p>
      <dl className="grid grid-cols-2 gap-2 text-sm">
        <div><dt>{t("columns.installed")}</dt><dd>{o.installedVersion}</dd></div>
        <div><dt>{t("columns.fixed")}</dt><dd>{o.fixedVersion || t("noFix")}</dd></div>
        <div><dt>{t("detail.completed")}</dt><dd>{format.dateTime(new Date(o.lastCompletedScanAt), { dateStyle: "medium", timeStyle: "short" })}</dd></div>
        <div><dt>{t("detail.latestAttempt")}</dt><dd>{t(`detail.attempt.${o.latestAttemptStatus}`)}</dd></div>
      </dl>
      {o.containers.length ? <ul className="flex flex-col gap-2">{o.containers.map((c) => {
        const update = o.updates.find((u) => u.containerId === c.id);
        return <li key={c.id} className="text-sm">
          <Link className="underline" to={`/containers/${encodeURIComponent(`${hostId}~${c.id}`)}`}>{c.name}</Link> <span className="font-mono text-xs">{c.id}</span> · {c.state}
          {c.project && <p>{t("detail.project", { project: c.project, service: c.service ?? "" })}</p>}
          <p>{update?.state === "available" ? t("detail.updateAvailable", { version: update.version || t("detail.versionUnknown") }) : update?.state === "none" ? t("detail.noUpdate") : t("detail.updateUnknown")}</p>
          {c.projectId ? <Link className="underline" to={`/stacks/${encodeURIComponent(`${hostId}~${c.projectId}`)}?tab=overview`}>{t("detail.openUpdates")}</Link> : <Link className="underline" to={`/containers/${encodeURIComponent(`${hostId}~${c.id}`)}`}>{t("detail.openUpdates")}</Link>}
        </li>;
      })}</ul> : <p className="text-sm text-muted-foreground">{t(usageAvailable ? "detail.noContainers" : "detail.usageUnavailable")}</p>}
      {!o.ignored && <FieldGroup><Field><FieldLabel htmlFor={reasonId}>{t("detail.reason")}</FieldLabel><Input id={reasonId} value={reason} onChange={(e) => setReason(e.target.value)} maxLength={1000} /></Field></FieldGroup>}
      <Button size="sm" variant="outline" disabled={busy || !capability.available} title={capability.reason} onClick={() => void act()}>{o.ignored ? t("detail.unignore") : t("detail.ignore")}</Button>
      {error && <p role="alert">{t("detail.actionFailed")}</p>}
    </CardContent>
  </Card>;
}

export function AdvisoryContent({ data, error, hosts, selected, onRetry }: { data?: AdvisoryRead; error?: unknown; hosts: Map<string, Host>; selected?: string | null; onRetry: () => void }) {
  const t = useTranslations("security"), format = useFormatter();
  if (error) return <QueryError error={error} onRetry={onRetry} />;
  if (!data) return <div aria-busy="true"><Skeleton className="h-36 w-full" /></div>;
  const representatives = data.hosts.flatMap((h) => h.detail?.advisory ? [{ hostId: h.hostId, advisory: h.detail.advisory }] : []).sort((a, b) => (b.advisory.lastModifiedDate ?? "").localeCompare(a.advisory.lastModifiedDate ?? "") || a.hostId.localeCompare(b.hostId));
  const representative = representatives[0], a = representative?.advisory;
  return <div className="flex min-w-0 flex-col gap-5 [overflow-wrap:anywhere]">
    {a && <section className="flex flex-col gap-3">
      <h2 className="font-semibold">{a.title || a.vulnerabilityId}</h2>
      {a.description && <p className="whitespace-pre-wrap text-sm">{a.description}</p>}
      <p className="text-xs text-muted-foreground">{t("detail.source", { host: hosts.get(representative.hostId)?.name ?? representative.hostId, image: a.sourceOccurrence.imageId, package: a.sourceOccurrence.package })}</p>
      <dl className="grid grid-cols-2 gap-2 text-sm">
        {a.cvss && <><div><dt>{t("detail.cvss2")}</dt><dd>{a.cvss.v2Score ?? "–"} {a.cvss.v2Vector}</dd></div><div><dt>{t("detail.cvss3")}</dt><dd>{a.cvss.v3Score ?? "–"} {a.cvss.v3Vector}</dd></div></>}
        <div><dt>{t("epss")}</dt><dd>{a.epss === undefined ? t("unknownIntel") : format.number(a.epss, { style: "percent", maximumFractionDigits: 2 })}</dd></div>
        <div><dt>{t("kev")}</dt><dd>{a.kev === undefined ? t("unknownIntel") : t(a.kev ? "detail.yes" : "detail.no")}</dd></div>
        {a.publishedDate && <div><dt>{t("detail.published")}</dt><dd>{format.dateTime(new Date(a.publishedDate), { dateStyle: "medium" })}</dd></div>}
        {a.lastModifiedDate && <div><dt>{t("detail.modified")}</dt><dd>{format.dateTime(new Date(a.lastModifiedDate), { dateStyle: "medium" })}</dd></div>}
      </dl>
      {!!a.references?.length && <ul className="flex flex-col gap-1 text-sm">{a.references.map((ref, i) => { const href = safeReference(ref); return <li key={i}>{href ? <a href={href} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 underline">{ref}<ExternalLink aria-hidden className="size-3" /></a> : <span>{ref}</span>}</li>; })}</ul>}
    </section>}
    {!data.hosts.length && <p>{t("detail.noHosts")}</p>}
    {data.hosts.map((h) => <section key={h.hostId} className="flex min-w-0 flex-col gap-3" aria-label={hosts.get(h.hostId)?.name ?? h.hostId}>
      <h2 className="font-semibold">{hosts.get(h.hostId)?.name ?? h.hostId}</h2>
      {!h.detail || h.code ? <Alert><AlertDescription>{t("detail.hostUnknown")}</AlertDescription></Alert> : <>
        {h.detail.usage.state === "unavailable" && <Alert><AlertDescription>{t("detail.usageUnavailable")}</AlertDescription></Alert>}
        {!h.detail.occurrences.length && <p>{t("detail.noOccurrences")}</p>}
        {h.detail.occurrences.map((o) => {
          const key = occurrenceSelectionKey(selectionFor(h.hostId, o));
          // Unknown usage must not imply an empty set of consumers.
          return <Occurrence key={key} hostId={h.hostId} occurrence={o} selected={selected === key} usageAvailable={h.detail!.usage.state === "ok"} />;
        })}
      </>}
    </section>)}
  </div>;
}
