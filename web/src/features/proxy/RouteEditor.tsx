// Source: web/src/features/design-lab/proxy/RouteEditor.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Lab section, step and segmented-mode structure on the production editor (issue 473 write path): only changed leaves, held read, Caddy Advanced authority, acknowledgement gating.

import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useTranslations } from "use-intl";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Textarea } from "@/shared/ui/textarea";
import { Label } from "@/shared/ui/label";
import { Input } from "@/shared/ui/input";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { StepRail } from "@/shared/components/step-rail";
import { Code, Wand2 } from "lucide-react";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { AcknowledgementFields, RefusalView, ConfirmCheck, ProxyDialog } from "./controls";
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

import { useMediaQuery } from "@/shared/hooks/use-media-query";
import { DestinationFields } from "./DestinationFields";
import { HandlingFields } from "./HandlingFields";
import { TlsFields, PRESERVE } from "./TlsFields";
import { FieldHelp, FormSection } from "./presentation";
import { simpleHost, usedDomains } from "./suggestions";

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
  const narrow = useMediaQuery("(max-width: 767px)");
  const [step, setStep] = useState(0), [invalid, setInvalid] = useState("");
  const form = useRef<HTMLFormElement>(null);
  useLayoutEffect(() => {
    if (invalid) form.current?.querySelector<HTMLElement>(`#${invalid}`)?.focus();
  }, [invalid, step, mode]);
  const [name, setName] = useState(route?.name ?? ""),
    [domain, setDomain] = useState(
      caddy ? (route?.rule ?? "") : simpleHost(route?.rule ?? "") ?? "",
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
  const exactHostRule = !rule || simpleHost(rule) !== undefined;
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
  const acknowledgementsValid = acknowledgementsReady(inventory, caddy ? [] : unresolved, ack);
  const domains = usedDomains(read.routes ?? [], route);
  const duplicate = !caddy && domains.includes(domain);
  const sectionTitles = [t("domainGroup"), t("destination"), t("tlsGroup"), t("handlingGroup")];
  const invalidField = () => {
    if (!name.trim()) return { id: "route-name", step: 0 };
    if (caddy && (mode === "advanced" || textChanged)) return !advanced.trim() ? { id: "site-block", step: 0 } : undefined;
    if (!(caddy ? domain.trim() : mode === "advanced" || !exactHostRule ? rule.trim() : domain.trim()))
      return { id: !caddy && (mode === "advanced" || !exactHostRule) ? "route-rule" : "route-domain", step: 0 };
    if (!opaqueService && !shared && !backend.host.trim()) return { id: "backend-host", step: 1 };
    if (!caddy && resolver !== PRESERVE && !resolver) return { id: "route-resolver", step: 2 };
    return undefined;
  };
  const save = () => {
    if (!acknowledgementsValid || busy) return;
    const missing = invalidField();
    if (missing) {
      if (["backend-host", "route-resolver"].includes(missing.id)) setMode("easy");
      else if (missing.id === "route-rule" || missing.id === "site-block") enterMode("advanced");
      setStep(missing.step);
      setInvalid(missing.id);
      // Repeated submission must focus again even when the error has not changed.
      form.current?.querySelector<HTMLElement>(`#${missing.id}`)?.focus();
      return;
    }
    setInvalid("");
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
  const section = (index: number, content: ReactNode) => (
    <div hidden={mode !== "easy" || (narrow && step !== index)}>
      <FormSection disabled={caddy && textChanged} title={sectionTitles[index]!} tone={(["domain", "target", "tls", "behavior"] as const)[index]!}>{content}</FormSection>
    </div>
  );
  return (
    <ProxyDialog wide title={t(route ? "edit" : "newRoute")} description={t("serverValidation")} onClose={close} busy={busy}>
      <form ref={form} noValidate onSubmit={e => {
        e.preventDefault();
        if (narrow && mode === "easy" && step < 3) setStep(s => s + 1);
        else save();
      }} className="grid min-w-0 gap-4">
        <fieldset disabled={busy} className="grid min-w-0 gap-4">
          <TooltipProvider><SegmentedToggle label={t("editorMode")} value={mode as "easy" | "advanced"} onChange={enterMode} options={[
            { value: "easy", label: t("easy"), icon: <Wand2 />, hint: t("easyHint") },
            { value: "advanced", label: t("advanced"), icon: <Code />, hint: t("advancedHint") },
          ]} /></TooltipProvider>
          {narrow && mode === "easy" && <>
            <StepRail label={t("editorSteps")} steps={sectionTitles.map((title, i) => ({ id: String(i), title }))} current={String(step)} reachable={3} onSelect={id => setStep(Number(id))} />
            <p role="status" className="sr-only">{t("step", { current: step + 1, total: 4 })} · {sectionTitles[step]}</p>
          </>}
          <div hidden={mode === "easy" && narrow && step !== 0}>
            <FieldHelp label={t("name")} help={t("help.name")}><TextField id="route-name" label={t("name")} disabled={caddy && textChanged} value={name} onChange={setName} /></FieldHelp>
          </div>
          <fieldset disabled={caddy && textChanged} className="grid min-w-0 gap-4">
            {section(0, <>
              <FieldHelp label={t("domain")} help={t(caddy ? "help.caddyDomain" : "help.domain")}>
                <div className="grid min-w-0 gap-2">
                  <Label htmlFor="route-domain">{t("domain")}</Label>
                  {/* Known domains are offered as a datalist (AUTO-17); picking one only fills the field. */}
                  <Input id="route-domain" value={domain} disabled={!caddy && !exactHostRule} list={!caddy && domains.length > 0 ? "route-domain-suggestions" : undefined} autoComplete="off" spellCheck={false} onChange={e => { const v = e.target.value; setDomain(v); if (!caddy) setRule(`Host(\`${v}\`)`); }} />
                  {!caddy && domains.length > 0 && <datalist id="route-domain-suggestions" aria-label={t("domainSuggestions")}>{domains.map(d => <option key={d} value={d} />)}</datalist>}
                </div>
              </FieldHelp>
              {!caddy && !exactHostRule && <p className="break-all font-mono text-xs">{rule}</p>}
            </>)}
            {section(1, opaqueService ? <p className="text-sm">{t("opaqueService", { name: route!.serviceFile! })}</p> : shared ? <p className="text-sm">{t("sharedServiceHint")}</p> : <DestinationFields instance={instance} backend={backend} update={setBackend} />)}
            {section(2, caddy ? <>
              <p className="text-sm text-muted-foreground">{t("caddyEasyHint")}</p>
              {!route && <FieldHelp label={t("tls")} help={t("help.caddyTls")}><ConfirmCheck checked={tls} onChange={setTls}>{t("tls")}</ConfirmCheck></FieldHelp>}
            </> : mode === "easy" ? <TlsFields inventory={inventory} route={route} resolver={resolver} setResolver={setResolver} entryPoints={entryPoints} setEntryPoints={setEntryPoints} /> : null)}
            {section(3, caddy ? <p className="text-sm text-muted-foreground">{t("caddyHandlingHint")}</p> : mode === "easy" ? <HandlingFields read={read} route={route} names={names} middlewares={middlewares} frozen={multiline} update={v => { setMiddlewares(v); setAck(a => ({ ...a, unresolvedReferences: [] })); }} priority={priority} setPriority={setPriority} /> : null)}
          </fieldset>
          {mode === "advanced" && <FormSection title={t("advanced")} tone="domain">
            {caddy ? <FieldHelp label={t("siteBlock")} help={t("help.siteBlock")}><div className="grid min-w-0 gap-2"><Label htmlFor="site-block">{t("siteBlock")}</Label><Textarea id="site-block" className="min-h-64 font-mono text-xs" value={advanced} onChange={e => { setAdvanced(e.target.value); setTextChanged(true); }} /></div></FieldHelp> : <>
              <FieldHelp label={t("rule")} help={t("help.rule")}><TextField id="route-rule" label={t("rule")} value={rule} onChange={v => { setRule(v); setDomain(simpleHost(v) ?? ""); }} /></FieldHelp>
              <FieldHelp label={t("priority")} help={t("help.priority")}><TextField id="advanced-priority" label={t("priority")} type="number" value={priority} onChange={setPriority} /></FieldHelp>
              <TlsFields inventory={inventory} route={route} resolver={resolver} setResolver={setResolver} entryPoints={entryPoints} setEntryPoints={setEntryPoints} />
            </>}
          </FormSection>}
          {duplicate && <p role="status" className="text-sm text-warning">{t("domainInUse", { domain })}</p>}
          {(!narrow || mode === "advanced" || step === 3) && <AcknowledgementFields inventory={inventory} unresolved={caddy ? [] : unresolved} value={ack} onChange={setAck} />}
        </fieldset>
        {invalid && <p role="alert">{t("requiredField", { name: t(({ "route-name": "name", "route-domain": "domain", "route-rule": "rule", "backend-host": "backendHost", "site-block": "siteBlock", "route-resolver": "resolver" } as const)[invalid as "route-name"] ?? "name") })}</p>}
        <RefusalView refusal={refusal} />
        <div className="sticky -bottom-6 z-10 -mx-6 -mb-6 flex flex-wrap justify-end gap-2 border-t bg-background px-6 py-3">
          <Button type="button" variant="outline" disabled={busy} onClick={close}>{t("cancel")}</Button>
          {narrow && mode === "easy" && <>
            <Button type="button" variant="outline" disabled={busy || step === 0} onClick={() => setStep(s => s - 1)}>{t("previous")}</Button>
            {step < 3 && <Button type="submit" disabled={busy}>{t("next")}</Button>}
          </>}
          {(!narrow || mode === "advanced" || step === 3) && <Button type="submit" disabled={!acknowledgementsValid || busy}>{t("save")}</Button>}
        </div>
      </form>
    </ProxyDialog>
  );
}
