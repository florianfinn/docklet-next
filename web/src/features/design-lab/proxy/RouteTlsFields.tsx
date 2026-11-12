// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Switch } from "@/shared/ui/switch";
import { ToneBadge } from "@/shared/components/tone";
import { OptionCards, SelectField, TextField } from "./controls";
import { matchingCertificates, useProxy, wildcardDomain, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";

export function RouteTlsFields({ route, patch }: { route: ProxyRoute; patch: (value: Partial<ProxyRoute>) => void }) {
  const t = useProxyText();
  const { certificates, resolvers } = useProxy();
  const wildcard = wildcardDomain(route);
  const matches = matchingCertificates({ ...route, tls: "custom" }, certificates);
  const choices = resolvers.filter((r) => r.proxy === route.proxy && (wildcard === undefined || r.challenge === "dns"));
  return <>
    <SelectField label={t("sslSelection")} value={route.tls} onChange={(tls) => patch({ tls: tls as ProxyRoute["tls"], redirect: tls === "off" ? false : route.redirect })} options={[{ value: "off", label: t("noSsl") }, { value: "auto", label: t("requestSsl") }, { value: "custom", label: t("existingSsl") + (matches.length ? " · " + matches.map((c) => c.name).join(", ") : "") }]} />
    {route.tls === "auto" && <>
      {!route.domain.startsWith("*.") && <OptionCards label={t("newSslScope")} value={wildcard === undefined ? "single" : "wildcard"} onChange={(value) => patch({ certificateDomain: value === "wildcard" ? route.domain.split(".").slice(-2).join(".") : undefined })} options={[{ value: "single", label: t("singleDomain") }, { value: "wildcard", label: t("wildcardCertificate") }]} />}
      {wildcard !== undefined && !route.domain.startsWith("*.") && <TextField label={t("certificateBase")} value={wildcard} onChange={(certificateDomain) => patch({ certificateDomain })} required />}
      <Field><FieldLabel>{t("certificatePreview")}</FieldLabel><div className="flex flex-wrap gap-2">{(wildcard !== undefined ? [wildcard, wildcard ? "*." + wildcard : ""] : [route.domain]).filter(Boolean).map((domain) => <ToneBadge key={domain} tone={domain.startsWith("*.") ? "violet" : "ok"}>{domain}</ToneBadge>)}</div><FieldDescription>{t(wildcard !== undefined ? "wildcardHint" : "singleSslHint")}</FieldDescription></Field>
      <SelectField label={t("resolver")} value={route.resolver} onChange={(resolver) => patch({ resolver })} options={choices.map((r) => ({ value: r.name, label: r.name }))} />
      {!choices.length && <p className="text-sm text-warning">{t("dnsRequired")}</p>}
    </>}
    {route.tls === "custom" && <Field><FieldLabel>{t("matchingCert")}</FieldLabel><div className="flex flex-wrap gap-2">{matches.length ? matches.map((cert) => <ToneBadge key={cert.id} tone="ok">{cert.domains.join(", ")}</ToneBadge>) : <span className="text-sm text-warning">{t("noCert")}</span>}</div><FieldDescription>{t("routeSslHint")}</FieldDescription></Field>}
    <Field orientation="horizontal"><Switch id="proxy-redirect" checked={route.redirect} disabled={route.tls === "off"} onCheckedChange={(redirect) => patch({ redirect })} /><FieldLabel htmlFor="proxy-redirect">{t("redirect")}</FieldLabel></Field>
  </>;
}
