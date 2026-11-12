// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useProxyText } from "./use-proxy-text";
import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Award, FileKey2, Plus, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useFormatter } from "use-intl";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";
import { Input } from "@/shared/ui/input";
import { CertificateCollection } from "./CertificateCollection";
import { useProxyDesign } from "./design";
import { Field, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { ToneBadge, IconTile } from "@/shared/components/tone";
import { Detail, FormSection, OptionCards, SelectField, TextField } from "./controls";
import { coversDomain, proxyStore, useProxy, validDomain, type ProxyCertificate } from "./model";

function CertificateEditor({ initial, onClose }: { initial: ProxyCertificate; onClose: () => void }) {
  const t = useProxyText();
  const { resolvers } = useProxy();
  const [certificate, setCertificate] = useState(initial);
  const [domains, setDomains] = useState(initial.domains.join(", "));
  const [error, setError] = useState("");
  const patch = (value: Partial<ProxyCertificate>) => setCertificate((c) => ({ ...c, ...value }));
  const wildcard = domains.includes("*.");
  const baseDomain = domains.split(",").map((d) => d.trim()).find((d) => d.startsWith("*."))?.slice(2) ?? "";
  const choices = resolvers.filter((r) => r.proxy === initial.proxy && (!wildcard || r.challenge === "dns"));
  function save() {
    const names = domains.split(",").map((d) => d.trim().toLowerCase()).filter(Boolean);
    if (!certificate.name.trim() || !names.length || names.some((d) => !validDomain(d)) || (certificate.origin === "imported" && (!certificate.certFile || !certificate.keyFile)) || (certificate.origin === "automatic" && !choices.some((r) => r.name === certificate.resolver))) { setError(t("certValidation")); return; }
    const next = { ...certificate, domains: names, status: "valid" as const, managed: certificate.origin === "automatic", issuer: certificate.origin === "automatic" ? t("issuerAcme") : certificate.issuer || t("issuerExample") };
    proxyStore.set((s) => ({ ...s, certificates: s.certificates.some((c) => c.id === next.id) ? s.certificates.map((c) => c.id === next.id ? next : c) : [...s.certificates, next] }));
    toast.success(t("saved")); onClose();
  }
  return <Dialog open onOpenChange={(open) => !open && onClose()}><DialogContent className="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl">
    <DialogHeader className="border-b p-6 pr-12 text-left"><DialogTitle>{t(certificate.origin === "automatic" ? "requestCert" : "importCert")}</DialogTitle><DialogDescription>{t(certificate.origin === "automatic" ? "wildcardHint" : "importHint")}</DialogDescription></DialogHeader>
    <form id="proxy-certificate-form" onSubmit={(e) => { e.preventDefault(); save(); }} className="min-h-0 flex-1 overflow-auto p-6">
      <FieldGroup className="grid items-start gap-5 md:grid-cols-2">
        <FormSection title={t("certIdentity")} tone="domain">

        <TextField label={t("certName")} value={certificate.name} onChange={(name) => patch({ name })} required />
        <OptionCards label={t("certificateScope")} value={wildcard ? "wildcard" : "single"} onChange={(value) => {
          const base = baseDomain || domains.split(",")[0]?.trim() || "";
          setDomains(value === "wildcard" ? (base ? base + ", *." + base : "*.") : domains.split(",").filter((d) => !d.trim().startsWith("*.")).join(", "));
        }} options={[{ value: "single", label: t("singleDomain") }, { value: "wildcard", label: t("wildcardCertificate") }]} />
        {wildcard && <TextField label={t("certificateBase")} value={baseDomain} onChange={(domain) => setDomains(domain + ", *." + domain)} hint={t("wildcardSslHint")} required />}
        {wildcard ? <Field><FieldLabel>{t("certificatePreview")}</FieldLabel><div className="flex flex-wrap gap-2">{domains.split(",").map((d) => d.trim()).filter((d) => validDomain(d)).map((domain) => <ToneBadge key={domain} tone={domain.startsWith("*.") ? "violet" : "ok"}>{domain}</ToneBadge>)}</div></Field> : <TextField label={t("domains")} value={domains} onChange={setDomains} hint={t("domainsHint")} required />}
        </FormSection>
        <FormSection title={t("certMethod")} tone="tls">
        <OptionCards label={t("origin")} value={certificate.origin} onChange={(origin) => patch({ origin: origin as ProxyCertificate["origin"] })} options={[{ value: "automatic", label: t("automatic") }, { value: "imported", label: t("imported") }]} />
        {certificate.origin === "automatic" && wildcard && !choices.length && <p className="text-sm text-warning">{t("dnsRequired")}</p>}
        {certificate.origin === "automatic" ? <SelectField label={t("resolver")} value={certificate.resolver} onChange={(resolver) => patch({ resolver })} options={choices.map((r) => ({ value: r.name, label: r.name }))} /> : <>
          <TextField label={t("issuer")} value={certificate.issuer} onChange={(issuer) => patch({ issuer })} />
          <TextField label={t("expires")} value={certificate.expires} type="date" onChange={(expires) => patch({ expires })} required />
          <TextField label={t("certificatePath")} value={certificate.certFile} onChange={(certFile) => patch({ certFile })} placeholder={t("examples.certificate")} required />
          <TextField label={t("keyPath")} value={certificate.keyFile} onChange={(keyFile) => patch({ keyFile })} placeholder={t("examples.key")} required />
        </>}
        </FormSection>
      </FieldGroup>
      {error && <p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}
    </form>
    <DialogFooter className="flex-row justify-end border-t p-4"><Button variant="outline" onClick={onClose}>{t("cancel")}</Button><Button type="submit" form="proxy-certificate-form">{t("save")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
export function CertificatesView({ instance }: { instance: string }) {
  const t = useProxyText();
  const format = useFormatter();
  const design = useProxyDesign();
  const [query, setQuery] = useState("");
  const [params] = useSearchParams();
  const { certificates, routes, resolvers } = useProxy();
  const [editing, setEditing] = useState<ProxyCertificate | null>(null);
  const list = certificates.filter((c) => c.proxy === instance && (c.name + " " + c.domains.join(" ")).toLowerCase().includes(query.toLowerCase()));
  function add() { setEditing({ id: crypto.randomUUID(), proxy: instance, name: "", domains: [], issuer: "", expires: "2027-01-29", origin: "automatic", resolver: resolvers.find((r) => r.proxy === instance)?.name ?? "", certFile: "", keyFile: "", managed: true, status: "pending" }); }
  function renew(cert: ProxyCertificate) {
    proxyStore.set((s) => ({ ...s, certificates: s.certificates.map((c) => c.id === cert.id ? { ...c, expires: new Date(Date.now() + 90 * 86400000).toISOString().slice(0, 10), status: "valid" } : c) })); toast.success(t("saved"));
  }
  function remove(cert: ProxyCertificate) {
    proxyStore.set((s) => ({ ...s, certificates: cert.origin === "imported" ? s.certificates.filter((c) => c.id !== cert.id) : s.certificates.map((c) => c.id === cert.id ? { ...c, managed: false } : c) })); toast.success(t(cert.origin === "imported" ? "removed" : "automationStopped"));
  }
  return <>
    <div className="mb-5 flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap items-center gap-3"><p className="text-sm text-muted-foreground">{t("certCount", { count: list.length })}</p>{design !== "classic" && <Input type="search" className="w-64" aria-label={t("certSearch")} placeholder={t("certSearch")} value={query} onChange={(e) => setQuery(e.target.value)} />}</div><div className="flex flex-wrap gap-2"><Button onClick={() => add()}><Plus data-icon="inline-start" />{t("addCert")}</Button></div></div>
    {design !== "classic" ? <CertificateCollection certificates={list} routes={routes} layout={design} selectedId={params.get("certificate")} onEdit={setEditing} onRenew={renew} onRemove={remove} /> : <div className="grid gap-5 lg:grid-cols-2 2xl:grid-cols-3">{list.map((cert) => {
      const linked = routes.filter((r) => r.proxy === instance && r.tls !== "off" && coversDomain(cert.domains, r.domain));
      return <Card id={"certificate-" + cert.id} key={cert.id} className={"gap-0 overflow-hidden py-0 " + (params.get("certificate") === cert.id ? "ring-1 ring-primary" : "")}>
        <CardHeader className="border-b p-5"><div className="flex items-center gap-3"><IconTile tone={cert.status === "expiresSoon" ? "warn" : "ok"}><Award /></IconTile><div className="min-w-0 flex-1"><CardTitle className="break-all text-base">{cert.name}</CardTitle><CardDescription className="mt-1">{cert.issuer}</CardDescription></div><ToneBadge tone={cert.status === "expiresSoon" ? "warn" : "ok"}>{t(cert.status)}</ToneBadge></div></CardHeader>
        <CardContent className="flex-1 p-5"><div className="flex flex-wrap gap-2">{cert.domains.map((domain) => <ToneBadge key={domain} tone={domain.startsWith("*.") ? "violet" : "gray"}>{domain}</ToneBadge>)}</div><dl className="mt-3"><Detail label={t("expires")}>{format.dateTime(new Date(cert.expires + "T12:00:00Z"), { dateStyle: "medium" })}</Detail><Detail label={t("origin")}>{t(cert.origin)}{cert.origin === "automatic" && !cert.managed && <p className="text-xs text-warning">{t("automationStopped")}</p>}</Detail><Detail label={t(cert.origin === "automatic" ? "resolver" : "certificatePath")}><span className="font-mono text-xs">{cert.origin === "automatic" ? cert.resolver : cert.certFile}</span></Detail></dl>
        <p className="mb-2 mt-4 text-xs font-medium text-muted-foreground">{t("usedBy")}</p><div className="flex flex-col gap-1">{linked.map((r) => <Link key={r.id} className="truncate text-sm text-primary-text hover:underline" to={"/proxy?route=" + r.id + "&instance=" + instance}>{r.domain}</Link>)}{!linked.length && <span className="text-sm text-muted-foreground">{t("noUsage")}</span>}</div></CardContent>
        <CardFooter className="flex-wrap gap-2 border-t p-4">
          <Button size="sm" variant="outline" onClick={() => setEditing(cert)}><FileKey2 data-icon="inline-start" />{t(cert.origin === "imported" ? "replace" : "edit")}</Button>
          {cert.origin === "automatic" && cert.managed && <Button size="sm" variant="outline" onClick={() => renew(cert)}><RefreshCw data-icon="inline-start" />{t("renew")}</Button>}
          {(cert.origin === "imported" || cert.managed) && <Button size="sm" variant="ghost" title={cert.origin === "automatic" ? t("automationHint") : undefined} onClick={() => remove(cert)}><Trash2 data-icon="inline-start" />{t(cert.origin === "imported" ? "removeImport" : "stopAutomation")}</Button>}
        </CardFooter>
      </Card>;
    })}</div>}
    {editing && <CertificateEditor key={editing.id} initial={editing} onClose={() => setEditing(null)} />}
  </>;
}
