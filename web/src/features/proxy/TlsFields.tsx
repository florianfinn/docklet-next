// Source: docklet-next/issues/549
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Contract-preserving TLS choices and inherited listener TLS explanation. Entry points stay editable under Keep current (AUTO-16).

import { useTranslations } from "use-intl";
import { ChoiceCards } from "@/shared/components/form-field";
import { Choice, ConfirmCheck } from "./controls";
import { FieldHelp } from "./presentation";
import type { Inventory, Route } from "./types";

// A NUL cannot occur in a Traefik resolver name (YAML key).
export const PRESERVE = "\u0000preserve";
export function TlsFields({ inventory, route, resolver, setResolver, entryPoints, setEntryPoints }: {
  inventory: Inventory; route?: Route; resolver: string; setResolver: (s: string) => void;
  entryPoints: string[]; setEntryPoints: (f: (old: string[]) => string[]) => void;
}) {
  const t = useTranslations("proxyContract.ui");
  const implicit = !entryPoints.length;
  const entryTLS = (inventory.entryPoints ?? []).some(ep => ep.tls && entryPoints.includes(ep.name));
  const currentImplicit = !(route?.entryPoints ?? []).length;
  const currentEntryTLS = (inventory.entryPoints ?? []).some(ep => ep.tls && (currentImplicit || route?.entryPoints?.includes(ep.name)));
  return <>
    <FieldHelp label={t("tlsChoice")} help={t("help.tls")}>
      <ChoiceCards label={t("tlsChoice")} value={resolver === PRESERVE ? "keep" : "resolver"} onChange={v => setResolver(v === "keep" ? PRESERVE : (route?.resolver || inventory.resolvers?.[0] || ""))} options={[
        { value: "keep", title: t(route ? "keepCurrent" : "noRouteCertificate"), description: t(route ? "keepCurrentHint" : "noRouteCertificateHint") },
        { value: "resolver", title: t("certificateFromResolver"), description: t("resolverChoiceHint") },
      ]} />
    </FieldHelp>
    <FieldHelp label={t("resolver")} help={t("help.resolver")}>
      <Choice id="route-resolver" label={t("resolver")} value={resolver} onChange={setResolver} options={[
        { value: PRESERVE, label: t(route ? "keepCurrent" : "noRouteCertificate") },
        ...(inventory.resolvers ?? []).map(r => {
          const summary = inventory.resolverSummaries?.find(s => s.name === r);
          return { value: r, label: summary ? `${r} · ${summary.challenge.kind.toUpperCase()}-01` : r };
        }),
      ]} />
    </FieldHelp>
    {route && <p className="text-xs text-muted-foreground">{route.resolver ? t("tlsResolver", { name: route.resolver }) : t(route.tls ? "tlsOn" : !currentEntryTLS ? "tlsOff" : currentImplicit ? "tlsDefaultEntryPoints" : "tlsEntryPoint")}</p>}
    {!route && <p className="text-xs text-muted-foreground">{t(implicit ? "defaultEntryPointsHint" : entryTLS ? "staticEntryPointsHint" : "plainEntryPointsHint")}</p>}
    <FieldHelp label={t("entryPoints")} help={t("help.entryPoints")}>
      {/* Entry points are their own leaf: "Keep current" governs only the resolver (AUTO-16). */}
      <fieldset className="grid gap-2"><legend className="mb-2 text-sm font-medium">{t("entryPoints")}</legend>
        {(inventory.entryPoints ?? []).map(ep => <ConfirmCheck key={ep.name} checked={entryPoints.includes(ep.name)} onChange={v => setEntryPoints(old => v ? [...old, ep.name] : old.filter(n => n !== ep.name))}>{ep.name} · {ep.address}</ConfirmCheck>)}
      </fieldset>
    </FieldHelp>
  </>;
}
