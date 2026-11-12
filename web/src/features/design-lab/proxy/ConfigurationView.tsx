// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { instanceKind } from "./route-text";
import { RouteEditor } from "./RouteEditor";
import { useProxyText } from "./use-proxy-text";
import { useState } from "react";
import { Pencil, Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { FieldGroup } from "@/shared/ui/field";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { ToneBadge } from "@/shared/components/tone";
import { Choice, FormSection, OptionCards, TextField } from "./controls";
import { proxyStore, useProxy, type Resolver } from "./model";

function ResolverEditor({ initial, onClose }: { initial: Resolver; onClose: () => void }) {
  const t = useProxyText();
  const caddy = instanceKind(initial.proxy) === "caddy";
  const [resolver, setResolver] = useState(initial);
  const [error, setError] = useState("");
  const patch = (change: Partial<Resolver>) => setResolver((r) => ({ ...r, ...change }));
  return <Dialog open onOpenChange={(open) => !open && onClose()}><DialogContent className="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl">
    <DialogHeader className="border-b p-6 pr-12 text-left"><DialogTitle>{t(caddy ? "editAcmeIssuer" : "editResolver")}</DialogTitle><DialogDescription>{t(caddy ? "acmeIssuerSubtitle" : "resolverSubtitle")}</DialogDescription></DialogHeader>
    <form id="proxy-resolver-form" className="flex-1 overflow-auto p-6" onSubmit={(e) => {
      e.preventDefault();
      if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$/.test(resolver.name) || !resolver.email.includes("@") || (caddy && resolver.challenge === "dns" && (!/^[a-z0-9_]+$/.test(resolver.provider) || !/^[a-z_][a-z0-9_]*$/i.test(resolver.credential)))) { setError(t(caddy ? "issuerInvalid" : "resolverValidation")); return; }
      proxyStore.set((s) => ({ ...s, resolvers: s.resolvers.some((r) => r.id === resolver.id) ? s.resolvers.map((r) => r.id === resolver.id ? resolver : r) : [...s.resolvers, resolver],
        routes: s.routes.map((r) => r.proxy === resolver.proxy && r.resolver === initial.name ? { ...r, resolver: resolver.name, caddyTls: undefined } : r),
        certificates: s.certificates.map((c) => c.proxy === resolver.proxy && c.resolver === initial.name ? { ...c, resolver: resolver.name } : c),
      }));
      toast.success(t("saved")); onClose();
    }}><FieldGroup>
      <FormSection title={t("resolverTitle")} tone="tls">
      <TextField label={t(caddy ? "acmeIssuerName" : "resolverName")} value={resolver.name} onChange={(name) => patch({ name })} required />
      <TextField label={t("email")} value={resolver.email} onChange={(email) => patch({ email })} type="email" required />
      <OptionCards label={t("challenge")} value={resolver.challenge} onChange={(challenge) => patch({ challenge: challenge as Resolver["challenge"] })} options={[{ value: "dns", label: t("dns") }, { value: "http", label: t("http") }]} />
      {resolver.challenge === "dns" && <><TextField label={t("provider")} value={resolver.provider} onChange={(provider) => patch({ provider })} placeholder={t("examples.provider")} /><TextField label={t("credential")} value={resolver.credential} onChange={(credential) => patch({ credential })} hint={t("credentialHint")} placeholder={t("examples.credential")} /></>}
      </FormSection>
    </FieldGroup>{error && <p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}</form>
    <DialogFooter className="flex-row justify-end border-t p-4"><Button variant="outline" onClick={onClose}>{t("cancel")}</Button><Button type="submit" form="proxy-resolver-form">{t("save")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
export function ConfigurationView({ instance }: { instance: string }) {
  const t = useProxyText();
  const { resolvers, routes } = useProxy();
  const caddy = instanceKind(instance) === "caddy";
  const [editRoute, setEditRoute] = useState(false);
  const [editing, setEditing] = useState<Resolver | null>(null);
  const [selected, setSelected] = useState("");
  const files = routes.filter((r) => r.proxy === instance && r.source === "file");
  const route = files.find((r) => r.id === selected) ?? files[0];
  return <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(20rem,1fr)]">
    <Card><CardHeader><CardTitle>{t("dynamicConfig")}</CardTitle><CardDescription>{t("configTextHint")}</CardDescription></CardHeader><CardContent>{route ? <><div className="mb-4 flex flex-wrap items-center gap-3"><ToneBadge tone="gray">{t(caddy ? "caddyfile" : "fileName")}</ToneBadge><Choice label={t("routes")} value={route.id} onChange={setSelected} options={files.map((r) => ({ value: r.id, label: r.domain }))} /></div><Button variant="outline" onClick={() => setEditRoute(true)}><Pencil data-icon="inline-start" />{t("editRoute")}</Button>{editRoute && <RouteEditor initial={route} onClose={() => setEditRoute(false)} />}</> : <p className="text-sm text-muted-foreground">{t("configNoRoutes")}</p>}</CardContent></Card>
    <Card><CardHeader><CardTitle>{t("resolverTitle")}</CardTitle><CardDescription>{t(caddy ? "acmeIssuerSubtitle" : "resolverSubtitle")}</CardDescription></CardHeader><CardContent className="flex flex-col gap-3">
      {resolvers.filter((r) => r.proxy === instance).map((resolver) => <div key={resolver.id} className="flex items-center gap-3 rounded-lg border p-4"><div className="min-w-0 flex-1"><p className="font-medium">{resolver.name}</p><p className="mt-1 text-xs text-muted-foreground">{t(resolver.challenge)}{resolver.provider && " · " + resolver.provider}</p></div><Button size="icon-sm" variant="ghost" aria-label={t(caddy ? "editAcmeIssuer" : "editResolver")} onClick={() => setEditing(resolver)}><Pencil /></Button></div>)}
      <Button variant="outline" onClick={() => setEditing({ id: crypto.randomUUID(), proxy: instance, name: "", email: "", challenge: "dns", provider: "", credential: "" })}><Plus data-icon="inline-start" />{t(caddy ? "addAcmeIssuer" : "addResolver")}</Button>
    </CardContent></Card>
    {editing && <ResolverEditor key={editing.id} initial={editing} onClose={() => setEditing(null)} />}
  </div>;
}
