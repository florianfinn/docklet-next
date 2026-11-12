// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useContainers } from "@/data/queries";
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Switch } from "@/shared/ui/switch";
import { Textarea } from "@/shared/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/shared/ui/toggle-group";
import { FormSection, OptionCards, SelectField, TextField } from "./controls";
import { instanceKind } from "./route-text";
import { useLab } from "./lab-bridge";
import { MiddlewareHelp } from "./MiddlewareHelp";
import { MIDDLEWARES, routeName, useProxy, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
export type RoutePatch = (change: Partial<ProxyRoute>) => void;
export function AddressFields({ route, patch, showPath = true }: { route: ProxyRoute; patch: RoutePatch; showPath?: boolean }) {
  const t = useProxyText();
  const { connections } = useProxy();
  const { info } = useLab();
  const lab = info?.proxies.find((p) => p.kind === connections[route.proxy]?.lab);
  return <FormSection title={t("addressGroup")} tone="domain"><TextField label={t("routeName")} value={route.name ?? routeName(route)} onChange={(name) => patch({ name })} hint={t(instanceKind(route.proxy) === "caddy" ? "caddyNameHint" : "routeNameHint")} required /><TextField label={t("domain")} value={route.domain} onChange={(domain) => patch({ domain })} suggestions={lab?.hostnames} placeholder={t("examples.domain")} hint={t(route.domain.startsWith("*.") ? "wildcardDomainHint" : "singleDomainHint")} required />{showPath && <TextField label={t("path")} value={route.path} onChange={(path) => patch({ path })} placeholder={t("examples.path")} />}</FormSection>;
}
export function TargetFields({ route, patch, kind, setKind, additional, setAdditional, split = false }: { route: ProxyRoute; patch: RoutePatch; kind: string; setKind: (kind: string) => void; additional: string; setAdditional: (value: string) => void; split?: boolean }) {
  const t = useProxyText();
  const containers = useContainers();
  const { connections } = useProxy();
  const { info } = useLab();
  const lab = connections[route.proxy]?.lab && info;
  const parts = route.url.match(/^(https?):\/\/([^/:]*)(?::([^/]*))?(.*)$/);
  const scheme = parts?.[1] ?? "http", host = parts?.[2] ?? "", port = parts?.[3] ?? "", path = parts?.[4] ?? "";
  const update = (next: { scheme?: string; host?: string; port?: string }) => patch({ url: (next.scheme ?? scheme) + "://" + (next.host ?? host) + ((next.port ?? port) ? ":" + (next.port ?? port) : "") + path });
  return <FormSection title={t("destinationGroup")} tone="target">
    <OptionCards label={t("targetType")} value={kind} onChange={(value) => { setKind(value); if (value === "external") patch({ containerId: undefined, stackId: undefined }); }} options={[{ value: "container", label: t("containerTarget") }, { value: "external", label: t("externalTarget") }]} />
    {kind === "container" && <SelectField label={t("chooseContainer")} value={route.containerId ?? ""} onChange={(id) => {
      const c = containers.data?.find((c) => c.id === id);
      if (c) patch({ containerId: c.id, stackId: c.stackId, hostId: c.hostId, url: "http://" + (c.serviceName ?? c.name) + ":" + (c.ports[0]?.container ?? "80") });
    }} options={(containers.data ?? []).filter((c) => !c.id.startsWith("sys-")).map((c) => ({ value: c.id, label: (c.stackId ? c.stackId + " / " : "") + (c.serviceName ?? c.name) }))} />}
    {split ? <div className="grid items-end gap-3 sm:grid-cols-[6rem_minmax(0,1fr)_7rem]"><SelectField label={t("scheme")} value={scheme} onChange={(scheme) => update({ scheme })} options={[{ value: "http", label: t("httpProtocol") }, { value: "https", label: t("httpsProtocol") }]} /><TextField label={t("forwardHost")} value={host} onChange={(host) => update({ host })} required /><TextField label={t("forwardPort")} value={port} onChange={(port) => update({ port })} /></div> : <TextField label={t("url")} value={route.url} onChange={(url) => patch({ url })} placeholder={t("examples.url")} required />}
    {lab && <SelectField label={t("labDestination")} value={lab.backends.includes(route.url) ? route.url : ""} onChange={(url) => { setKind("external"); patch({ url, containerId: undefined, stackId: undefined }); }} options={lab.backends.map((url) => ({ value: url, label: url }))} />}
    <details><summary className="cursor-pointer text-xs text-muted-foreground">{t("backends")}</summary><Field className="mt-3"><FieldLabel htmlFor="proxy-backends">{t("backends")}</FieldLabel><Textarea id="proxy-backends" value={additional} onChange={(e) => setAdditional(e.target.value)} rows={2} /><FieldDescription>{t("backendsHint")}</FieldDescription></Field></details>
  </FormSection>;
}
export function BehaviorFields({ route, patch, showPath = false }: { route: ProxyRoute; patch: RoutePatch; showPath?: boolean }) {
  const t = useProxyText();
  const caddy = instanceKind(route.proxy) === "caddy";
  const unsupported = caddy ? route.middlewares.filter((m) => !MIDDLEWARES.includes(m)) : [];
  return <FormSection title={t("behaviorGroup")} tone="behavior">
    {showPath && <TextField label={t("path")} value={route.path} onChange={(path) => patch({ path })} placeholder={t("examples.path")} />}
    <Field><FieldLabel>{t("middlewares")}</FieldLabel><ToggleGroup type="multiple" variant="outline" spacing={2} value={route.middlewares} onValueChange={(middlewares) => patch({ middlewares })} className="flex-wrap justify-start">{MIDDLEWARES.map((name) => <MiddlewareHelp key={name} name={name}><ToggleGroupItem value={name} aria-label={name}>{name}{route.middlewares.includes(name) && <span className="opacity-60">{route.middlewares.indexOf(name) + 1}</span>}</ToggleGroupItem></MiddlewareHelp>)}</ToggleGroup><FieldDescription>{t(caddy ? "caddyMiddlewareHint" : "middlewareHint")}</FieldDescription>{!!unsupported.length && <p role="alert" className="text-sm text-warning">{t("middlewareUnsupported")}: {unsupported.join(", ")}<button type="button" className="ml-2 underline" onClick={() => patch({ middlewares: route.middlewares.filter((m) => MIDDLEWARES.includes(m)) })}>{t("remove")}</button></p>}</Field>
    <Field orientation="horizontal"><Switch id="proxy-enabled" checked={route.enabled} onCheckedChange={(enabled) => patch({ enabled })} /><FieldLabel htmlFor="proxy-enabled">{t("enable")}</FieldLabel></Field>
  </FormSection>;
}
