// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { useQuery } from "@tanstack/react-query";
import { Globe, HouseWifi, Network } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useTailnet } from "@/data/use-tailnet";
import { tailnetRouteReason, type NetworkRoute } from "@/domain/tailnet";
import { installerControlUrl } from "@/domain/agent-install";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";
import { FieldSet, FieldLegend } from "@/shared/ui/field";
import { TextField } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import type { HubDiscovery } from "@/domain/hub-discovery";
import { patchDraft, type HostDraft } from "./onboarding-store";

export function EdgeRoute({ draft, locked, candidates }: { draft: HostDraft; locked: boolean; candidates?: HubDiscovery["candidates"] }) {
  const t = useTranslations("connections");
  const { api, status } = useTailnet();
  const settings = useQuery({ queryKey: ["tailnet-settings"], queryFn: () => api!.settings(), enabled: !!api, retry: false });
  const reason = tailnetRouteReason(status);
  const unusableControl = !!settings.data?.customControl && !installerControlUrl(settings.data.controlUrl);
  const disabled = reason !== undefined || !settings.data || unusableControl;
  const routes = [{ value: "lan", icon: HouseWifi, titleKey: "lan", hint: "lanHint" }, { value: "domain", icon: Globe, titleKey: "domain", hint: "domainHint" }, { value: "tailnet", icon: Network, titleKey: "name", hint: "routeHint" }] as const;
  const managerUrl = status?.hubAddress ? `http://${status.hubAddress}:3552` : "…";
  function select(value: string) { patchDraft({ route: value as NetworkRoute, hubUrl: value === "tailnet" ? undefined : candidates?.find((item) => item.kind === (value === "domain" ? "domain" : "ip"))?.url ?? "", tailnetAuthKey: undefined }); }
  return <FieldSet data-slot="tailnet-route" data-ready={!!status && !!settings.data} className="flex min-w-0 flex-col gap-3">
    <FieldLegend variant="label">{t("route")}</FieldLegend>
    <RadioGroup value={draft.route ?? "lan"} onValueChange={select} disabled={locked} aria-label={t("route")} className="grid min-w-0 gap-2">
      {routes.map(({ value, icon: Icon, titleKey, hint }) => <label key={value} htmlFor={`host-route-${value}`}
        className={cn("flex min-h-10 min-w-0 items-start gap-3 rounded-xl border p-3.5 text-left disabled:cursor-not-allowed disabled:opacity-60", (draft.route ?? "lan") === value ? "border-control/60 bg-control/12" : "bg-card/40", value === "tailnet" && disabled && "border-dashed")}>
        <RadioGroupItem id={`host-route-${value}`} value={value} disabled={value === "tailnet" && disabled} aria-describedby={value === "tailnet" && disabled ? "tailnet-route-reason" : undefined} className="mt-0.5" />
        <Icon className="mt-0.5 size-4 shrink-0" aria-hidden /><span className="min-w-0"><span className="flex flex-wrap items-center gap-2 text-sm font-medium">{t(titleKey)}{value === "tailnet" && <ToneBadge tone="teal">{t("private")}</ToneBadge>}</span><span className="mt-1 block text-xs text-muted-foreground">{t(hint)}</span></span>
      </label>)}
    </RadioGroup>
    {disabled && <p id="tailnet-route-reason" className="text-xs text-muted-foreground">{unusableControl && reason === undefined ? t("customControlUnusable") : t(`routeReasons.${reason ?? "checking"}`)} <Link to="/settings/connections" className="text-primary-text underline">{t("connectionsLink")}</Link></p>}
    {draft.route === "tailnet" && <div className="min-w-0 rounded-lg border p-3">
      <p className="font-mono text-sm break-all">{managerUrl}</p><p className="mt-1 text-xs text-muted-foreground">{t("fixedHub")}</p>
      {status?.mode === "manual" ? <TextField id="tailnet-host-key" type="password" label={t("manualHostKey")} value={draft.tailnetAuthKey ?? ""} onChange={(tailnetAuthKey) => patchDraft({ tailnetAuthKey })} description={t("manualHostHint")} inputClassName="min-h-10" className="mt-3" disabled={locked} /> : <p className="mt-3 text-sm">{t("automaticHostKey")}</p>}
    </div>}
  </FieldSet>;
}
