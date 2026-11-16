// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Textarea } from "@/shared/ui/textarea";
import { Label } from "@/shared/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { AcknowledgementFields, RefusalView, Choice, ConfirmCheck, ProxyDialog } from "./controls";
import {
  acknowledgementsReady,
  backendParts,
  backendURL,
  emptyAcknowledgements,
  relativeTarget,
  splitNames,
} from "./model";
import { newCaddySite, patchCaddySite } from "./caddy-draft";
import type { Acknowledgements, Change, Instance, Inventory, ReadResult, Route } from "./types";

// The "keep current TLS" choice; a NUL cannot occur in a Traefik resolver name (YAML key).
const PRESERVE = "\u0000preserve";

export function RouteEditor({
  instance,
  inventory,
  read,
  route,
  busy,
  refusal,
  submit,
  close,
}: {
  instance: Instance;
  inventory: Inventory;
  read: ReadResult;
  route?: Route;
  busy: boolean;
  refusal?: unknown;
  submit: (target: string, change: Change, ack: Acknowledgements) => void;
  close: () => void;
}) {
  const t = useTranslations("proxyContract.ui"),
    caddy = instance.kind === "caddy";
  const [mode, setMode] = useState("easy");
  const [name, setName] = useState(route?.name ?? ""),
    [domain, setDomain] = useState(
      caddy ? (route?.rule ?? "") : (route?.rule.match(/^Host\([`"]([^`"]+)[`"]\)$/)?.[1] ?? ""),
    );
  const [rule, setRule] = useState(route?.rule ?? ""),
    [priority, setPriority] = useState(String(route?.priority ?? 0));
  const [backend, setBackend] = useState(() => backendParts(route?.upstreams?.[0] ?? ""));
  const [entryPoints, setEntryPoints] = useState(route?.entryPoints ?? []);
  // One name per line: Traefik names may contain commas.
  const [middlewares, setMiddlewares] = useState((route?.references ?? []).map(r => r.name).join("\n"));
  const [resolver, setResolver] = useState(PRESERVE),
    [tls, setTls] = useState(true);
  const [ack, setAck] = useState(emptyAcknowledgements);
  const originalText = (read.files ?? []).find(f => f.path === route?.file)?.content ?? "";
  const [advanced, setAdvanced] = useState(originalText),
    [textChanged, setTextChanged] = useState(false);
  // Untouched backend parts keep the server's string byte for byte (also forms like an empty explicit port).
  const upstream =
    route?.upstreams?.[0] !== undefined && JSON.stringify(backend) === JSON.stringify(backendParts(route.upstreams[0]))
      ? route.upstreams[0]
      : backendURL(backend);
  // A router without entry points uses the default ones, which the inventory cannot name exactly.
  const implicitEntryPoints = !!route && !(route.entryPoints ?? []).length;
  const entryTLS = (inventory.entryPoints ?? []).some(
    ep => ep.tls && (implicitEntryPoints || (route?.entryPoints ?? []).includes(ep.name)),
  );
  const opaqueService = !!route && !route.upstreams?.length && !!route.serviceFile;
  const shared = !caddy && (route?.dependants?.length ?? 0) > 1;
  const known = [
    ...new Set([
      ...(read.middlewares ?? []).map(m => m.name),
      ...(read.routes ?? []).flatMap(r =>
        (r.references ?? []).filter(ref => !["opaque", "unresolved"].includes(ref.kind)).map(ref => ref.name),
      ),
    ]),
  ];
  // A name that is empty or contains a line break cannot be shown one per line; such a
  // list stays exactly as read and its field and picker are disabled.
  const multiline = (route?.references ?? []).some(r => r.name === "" || r.name.includes("\n"));
  const names = multiline ? (route?.references ?? []).map(r => r.name) : splitNames(middlewares);
  const unresolved = names.filter(n => !known.includes(n) && !route?.references?.some(r => r.name === n));
  const exactHostRule = !rule || /^Host\([`"]([^`"]+)[`"]\)$/.test(rule);
  const caddyText = route
    ? patchCaddySite(originalText, route, name, domain, upstream)
    : newCaddySite(name, domain, upstream, tls);
  const enterMode = (value: string) => {
    if (caddy && value === "advanced" && !textChanged) setAdvanced(caddyText);
    setMode(value);
  };
  const target =
    route?.file ??
    relativeTarget(instance, `${instance.routeDirectory.replace(/\/$/, "")}/${name}.${caddy ? "caddy" : "yml"}`);
  const valid =
    acknowledgementsReady(inventory, caddy ? [] : unresolved, ack) &&
    (caddy && mode === "advanced"
      ? !!name.trim() && !!advanced.trim()
      : !!name.trim() &&
        (caddy ? !!domain.trim() : mode === "advanced" || !exactHostRule ? !!rule.trim() : !!domain.trim()) &&
        (opaqueService || shared || !!backend.host.trim()));
  const save = () => {
    if (!valid || busy) return;
    let change: Change;
    if (caddy)
      change = {
        kind: route ? "edit-route" : "create-route",
        routeId: route?.id,
        advanced: mode === "advanced" || textChanged ? advanced : caddyText,
      };
    else {
      change = { kind: route ? "edit-route" : "create-route", ...(route ? { routeId: route.id } : {}) };
      if (!route || name !== route.name) change.name = name;
      if (!route || rule !== route.rule) change.rule = rule;
      // A new route omits empty defaults; an edit sends only changed leaves.
      if (route ? Number(priority) !== route.priority : Number(priority) !== 0) change.priority = Number(priority);
      if (route ? JSON.stringify(entryPoints) !== JSON.stringify(route.entryPoints ?? []) : entryPoints.length > 0)
        change.entryPoints = entryPoints;
      if (
        route
          ? JSON.stringify(names) !== JSON.stringify((route.references ?? []).map(ref => ref.name))
          : names.length > 0
      )
        change.middlewares = names;
      if (resolver !== PRESERVE) change.resolver = resolver;
      if (!opaqueService && !shared && (!route || upstream !== route.upstreams?.[0]))
        change.upstreams = [upstream, ...(route?.upstreams?.slice(1) ?? [])];
    }
    submit(target, change, ack);
  };
  return (
    <ProxyDialog title={t(route ? "edit" : "newRoute")} description={t("serverValidation")} onClose={close} busy={busy}>
      <form
        onSubmit={e => {
          e.preventDefault();
          save();
        }}
        className="grid min-w-0 gap-4"
      >
        <fieldset disabled={busy} className="grid min-w-0 gap-4">
          <TextField
            id="route-name"
            label={t("name")}
            disabled={caddy && textChanged}
            value={name}
            onChange={setName}
          />
          <Tabs value={mode} onValueChange={enterMode}>
            <TabsList>
              <TabsTrigger value="easy">{t("easy")}</TabsTrigger>
              <TabsTrigger value="advanced">{t("advanced")}</TabsTrigger>
            </TabsList>
            <TabsContent value="easy" className="grid min-w-0 gap-4 pt-3">
              <fieldset className="grid min-w-0 gap-4" disabled={caddy && textChanged}>
                <TextField
                  id="route-domain"
                  label={t("domain")}
                  value={domain}
                  disabled={!caddy && !exactHostRule}
                  onChange={v => {
                    setDomain(v);
                    if (!caddy) setRule(`Host(\`${v}\`)`);
                  }}
                />
                {!caddy && !exactHostRule && <p className="break-all font-mono text-xs">{rule}</p>}
                {opaqueService ? (
                  <p className="text-sm">{t("opaqueService", { name: route.serviceFile! })}</p>
                ) : shared ? (
                  <p className="text-sm">{t("sharedServiceHint")}</p>
                ) : (
                  <div className="grid min-w-0 gap-3 sm:grid-cols-[1fr_2fr_1fr]">
                    <Choice
                      id="backend-scheme"
                      label={t("scheme")}
                      value={backend.scheme}
                      onChange={v => setBackend(b => ({ ...b, scheme: v }))}
                      options={[...new Set(["http", "https", ...(backend.scheme ? [backend.scheme] : [])])].map(v => ({
                        value: v,
                        label: v,
                      }))}
                    />
                    <TextField
                      id="backend-host"
                      label={t("backendHost")}
                      value={backend.host}
                      onChange={v => setBackend(b => ({ ...b, host: v }))}
                    />
                    <TextField
                      id="backend-port"
                      label={t("port")}
                      value={backend.port}
                      onChange={v => setBackend(b => ({ ...b, port: v, colon: v !== "" }))}
                    />
                  </div>
                )}
                {!caddy && (
                  <>
                    <fieldset className="grid gap-2">
                      <legend className="mb-2 text-sm font-medium">{t("entryPoints")}</legend>
                      {(inventory.entryPoints ?? []).map(ep => (
                        <ConfirmCheck
                          key={ep.name}
                          checked={entryPoints.includes(ep.name)}
                          onChange={v =>
                            setEntryPoints(old => (v ? [...old, ep.name] : old.filter(n => n !== ep.name)))
                          }
                        >
                          {ep.name} · {ep.address}
                        </ConfirmCheck>
                      ))}
                    </fieldset>
                    <Choice
                      id="route-resolver"
                      label={t("resolver")}
                      value={resolver}
                      onChange={setResolver}
                      options={[
                        { value: PRESERVE, label: t("preserveTls") },
                        ...(inventory.resolvers ?? []).map(r => ({ value: r, label: r })),
                      ]}
                    />
                    {route && (
                      <p className="text-xs text-muted-foreground">
                        {route.resolver
                          ? t("tlsResolver", { name: route.resolver })
                          : t(
                              route.tls
                                ? "tlsOn"
                                : !entryTLS
                                  ? "tlsOff"
                                  : implicitEntryPoints
                                    ? "tlsDefaultEntryPoints"
                                    : "tlsEntryPoint",
                            )}
                      </p>
                    )}
                    <div className="grid min-w-0 gap-2">
                      <Label htmlFor="route-middlewares">{t("middlewares")}</Label>
                      <Textarea
                        id="route-middlewares"
                        className="min-h-20 font-mono text-xs"
                        value={middlewares}
                        disabled={multiline}
                        aria-describedby="route-middlewares-desc"
                        spellCheck={false}
                        onChange={e => {
                          setMiddlewares(e.target.value);
                          setAck(a => ({ ...a, unresolvedReferences: [] }));
                        }}
                      />
                      <p id="route-middlewares-desc" className="text-xs text-muted-foreground">
                        {t("middlewareHint")}
                      </p>
                    </div>
                    {known.length > 0 && (
                      <Choice
                        id="known-middleware"
                        label={t("existingMiddleware")}
                        value=""
                        placeholder={t("selectMiddleware")}
                        disabled={multiline}
                        onChange={v => {
                          if (!names.includes(v)) setMiddlewares([...names, v].join("\n"));
                        }}
                        options={known.filter(n => n !== "" && !n.includes("\n")).map(n => ({ value: n, label: n }))}
                      />
                    )}
                  </>
                )}
                {caddy && (
                  <>
                    <p className="text-sm text-muted-foreground">{t("caddyEasyHint")}</p>
                    {!route && (
                      <ConfirmCheck checked={tls} onChange={setTls}>
                        {t("tls")}
                      </ConfirmCheck>
                    )}
                  </>
                )}
              </fieldset>
            </TabsContent>
            <TabsContent value="advanced" className="grid min-w-0 gap-4 pt-3">
              {caddy ? (
                <div className="grid min-w-0 gap-2">
                  <Label htmlFor="site-block">{t("siteBlock")}</Label>
                  <Textarea
                    id="site-block"
                    className="min-h-64 font-mono text-xs"
                    value={advanced}
                    onChange={e => {
                      setAdvanced(e.target.value);
                      setTextChanged(true);
                    }}
                  />
                </div>
              ) : (
                <>
                  <TextField
                    id="route-rule"
                    label={t("rule")}
                    value={rule}
                    onChange={v => {
                      setRule(v);
                      setDomain(v.match(/^Host\([`"]([^`"]+)[`"]\)$/)?.[1] ?? "");
                    }}
                  />
                  <TextField
                    id="route-priority"
                    label={t("priority")}
                    type="number"
                    value={priority}
                    onChange={setPriority}
                  />
                  <Choice
                    id="advanced-resolver"
                    label={t("resolver")}
                    value={resolver}
                    onChange={setResolver}
                    options={[
                      { value: PRESERVE, label: t("preserveTls") },
                      ...(inventory.resolvers ?? []).map(r => ({ value: r, label: r })),
                    ]}
                  />
                </>
              )}
            </TabsContent>
          </Tabs>
          <AcknowledgementFields
            inventory={inventory}
            unresolved={caddy ? [] : unresolved}
            value={ack}
            onChange={setAck}
          />
        </fieldset>
        <RefusalView refusal={refusal} />
        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" variant="outline" disabled={busy} onClick={close}>
            {t("cancel")}
          </Button>
          <Button type="submit" disabled={!valid || busy}>
            {t("save")}
          </Button>
        </div>
      </form>
    </ProxyDialog>
  );
}
