import { useProxyText } from "./use-proxy-text";
import { useState } from "react";
import { Pencil, Plus, RotateCw, Save } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { FieldGroup } from "@/shared/ui/field";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Textarea } from "@/shared/ui/textarea";
import { ToneBadge } from "@/shared/components/tone";
import { Choice, FormSection, OptionCards, TextField } from "./controls";
import { routeNameConflict, proxyStore, wildcardProblem, routeFromYaml, routeYaml, upsertRoute, useProxy, type ProxyRoute, type Resolver } from "./model";

function ResolverEditor({ initial, onClose }: { initial: Resolver; onClose: () => void }) {
  const t = useProxyText();
  const [resolver, setResolver] = useState(initial);
  const [error, setError] = useState("");
  const patch = (change: Partial<Resolver>) => setResolver((r) => ({ ...r, ...change }));
  return <Dialog open onOpenChange={(open) => !open && onClose()}><DialogContent className="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl">
    <DialogHeader className="border-b p-6 pr-12 text-left"><DialogTitle>{t("editResolver")}</DialogTitle><DialogDescription>{t("resolverSubtitle")}</DialogDescription></DialogHeader>
    <form id="proxy-resolver-form" className="flex-1 overflow-auto p-6" onSubmit={(e) => {
      e.preventDefault();
      if (!resolver.name.trim() || !resolver.email.includes("@")) { setError(t("resolverValidation")); return; }
      proxyStore.set((s) => ({ ...s, resolvers: s.resolvers.some((r) => r.id === resolver.id) ? s.resolvers.map((r) => r.id === resolver.id ? resolver : r) : [...s.resolvers, resolver],
        routes: s.routes.map((r) => r.proxy === resolver.proxy && r.resolver === initial.name ? { ...r, resolver: resolver.name } : r),
        certificates: s.certificates.map((c) => c.proxy === resolver.proxy && c.resolver === initial.name ? { ...c, resolver: resolver.name } : c),
      }));
      toast.success(t("saved")); onClose();
    }}><FieldGroup>
      <FormSection title={t("resolverTitle")} tone="tls">
      <TextField label={t("resolverName")} value={resolver.name} onChange={(name) => patch({ name })} required />
      <TextField label={t("email")} value={resolver.email} onChange={(email) => patch({ email })} type="email" required />
      <OptionCards label={t("challenge")} value={resolver.challenge} onChange={(challenge) => patch({ challenge: challenge as Resolver["challenge"] })} options={[{ value: "dns", label: t("dns") }, { value: "http", label: t("http") }]} />
      {resolver.challenge === "dns" && <><TextField label={t("provider")} value={resolver.provider} onChange={(provider) => patch({ provider })} placeholder={t("examples.provider")} /><TextField label={t("credential")} value={resolver.credential} onChange={(credential) => patch({ credential })} hint={t("credentialHint")} placeholder={t("examples.credential")} /></>}
      </FormSection>
    </FieldGroup>{error && <p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}</form>
    <DialogFooter className="flex-row justify-end border-t p-4"><Button variant="outline" onClick={onClose}>{t("cancel")}</Button><Button type="submit" form="proxy-resolver-form">{t("save")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
function YamlEditor({ route }: { route: ProxyRoute }) {
  const { resolvers, routes } = useProxy();
  const t = useProxyText();
  const [yaml, setYaml] = useState(() => routeYaml(route));
  const [error, setError] = useState("");
  return <><Textarea aria-label={t("yaml")} value={yaml} onChange={(e) => setYaml(e.target.value)} className="min-h-96 font-mono text-xs leading-relaxed" spellCheck={false} />{error && <p role="alert" className="mt-3 text-sm text-destructive">{error}</p>}<div className="mt-4 flex flex-wrap gap-2"><Button onClick={() => {
    try { const candidate = routeFromYaml(yaml, route); if (routeNameConflict(candidate, routes)) { setError(t("routeNameTaken")); return; } const problem = wildcardProblem(candidate, resolvers); if (problem) { setError(t(problem)); return; } upsertRoute(candidate); setYaml(routeYaml(candidate)); setError(""); toast.success(t("saved")); } catch { setError(t("invalidYaml")); }
  }}><Save data-icon="inline-start" />{t("save")}</Button><Button variant="outline" onClick={() => toast.success(t("reloaded"))}><RotateCw data-icon="inline-start" />{t("reload")}</Button></div></>;
}
export function ConfigurationView({ instance }: { instance: string }) {
  const t = useProxyText();
  const { resolvers, routes } = useProxy();
  const [editing, setEditing] = useState<Resolver | null>(null);
  const [selected, setSelected] = useState("");
  const files = routes.filter((r) => r.proxy === instance && r.source === "file");
  const route = files.find((r) => r.id === selected) ?? files[0];
  return <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(20rem,1fr)]">
    <Card><CardHeader><CardTitle>{t("dynamicConfig")}</CardTitle><CardDescription>{t("configHint")}</CardDescription></CardHeader><CardContent>{route ? <><div className="mb-4 flex flex-wrap items-center gap-3"><ToneBadge tone="gray">{t("fileName")}</ToneBadge><Choice label={t("routes")} value={route.id} onChange={setSelected} options={files.map((r) => ({ value: r.id, label: r.domain }))} /></div><YamlEditor key={route.id} route={route} /></> : <p className="text-sm text-muted-foreground">{t("configNoRoutes")}</p>}</CardContent></Card>
    <Card><CardHeader><CardTitle>{t("resolverTitle")}</CardTitle><CardDescription>{t("resolverSubtitle")}</CardDescription></CardHeader><CardContent className="flex flex-col gap-3">
      {resolvers.filter((r) => r.proxy === instance).map((resolver) => <div key={resolver.id} className="flex items-center gap-3 rounded-lg border p-4"><div className="min-w-0 flex-1"><p className="font-medium">{resolver.name}</p><p className="mt-1 text-xs text-muted-foreground">{t(resolver.challenge)}{resolver.provider && " · " + resolver.provider}</p></div><Button size="icon-sm" variant="ghost" aria-label={t("editResolver")} onClick={() => setEditing(resolver)}><Pencil /></Button></div>)}
      <Button variant="outline" onClick={() => setEditing({ id: crypto.randomUUID(), proxy: instance, name: "", email: "", challenge: "dns", provider: "", credential: "" })}><Plus data-icon="inline-start" />{t("addResolver")}</Button>
    </CardContent></Card>
    {editing && <ResolverEditor key={editing.id} initial={editing} onClose={() => setEditing(null)} />}
  </div>;
}
