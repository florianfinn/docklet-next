// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { Check, Minus, Network, X } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useTailnet } from "@/data/use-tailnet";
import { tailnetChain, tailnetHostDown } from "@/domain/tailnet";
import type { Host } from "@/domain/model";
import { formatRelative } from "@/domain/format";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { cn } from "@/shared/lib/utils";

export function TailnetHeaderChip() {
  const t = useTranslations("connections");
  const wire = useTranslations("tailnetContract.connection");
  const { status } = useTailnet();
  if (!status?.enabled) return null;
  return <Button asChild variant="outline" className="min-h-10 max-w-full whitespace-normal text-left"><Link to="/settings/connections"><Network aria-hidden />{t("chip", { state: wire(status.connection.state), count: status.hostCount })}</Link></Button>;
}

export function TailnetBadge() {
  const t = useTranslations("connections");
  return <ToneBadge tone="teal"><Network className="size-3" aria-hidden />{t("name")}</ToneBadge>;
}

export function TailnetChain({ host, compact = false }: { host: Host; compact?: boolean }) {
  const wire = useTranslations("tailnetContract");
  const t = useTranslations("connections");
  const { status } = useTailnet();
  const chain = tailnetChain(host, status);
  return <ol aria-label={t("cause")} className={cn("grid min-w-0 gap-2", compact ? "text-xs" : "text-sm")}>
    {chain.map(({ link, state }, index) => {
      const Icon = state === "ok" ? Check : state === "failed" ? X : Minus;
      return <li key={link} data-link={link} data-state={state} className={cn("flex min-w-0 items-start gap-2 rounded-lg", !compact && "border p-3", state === "failed" ? "border-destructive/50 bg-destructive/5" : state === "unknown" ? "border-dashed text-muted-foreground" : "border-border")}>
        <Icon aria-hidden className={cn("mt-0.5 size-4 shrink-0", state === "failed" ? "text-destructive" : state === "ok" ? "text-success" : "text-muted-foreground")} />
        <div className="min-w-0 flex-1"><div className="flex flex-wrap gap-1.5"><span>{index + 1}. {t(`links.${link}`)}</span>{!compact && state === "failed" && <ToneBadge tone="err">{t("firstFailure")}</ToneBadge>}</div>
          {!compact && link === "peer" && state !== "unknown" && host.tailnet && <div className="mt-1 text-xs text-muted-foreground"><span>{wire(`peer.${host.tailnet.state}`)}</span> · <span>{t(`path.${["direct", "relay", "idle"].includes(host.tailnet.path) ? host.tailnet.path as "direct" | "relay" | "idle" : "unknown"}`)}</span>{host.tailnet.lastSeen && <p>{t("lastSeen", { time: formatRelative(host.tailnet.lastSeen) })}</p>}</div>}
        </div><span className="shrink-0 text-xs">{t(`linkStates.${state}`)}</span>
      </li>;
    })}
  </ol>;
}

export function TailnetStatusTooltip({ host, children }: { host: Host; children: ReactNode }) {
  const t = useTranslations("connections");
  if (host.networkRoute !== "tailnet" || !tailnetHostDown(host)) return children;
  return <Tooltip><TooltipTrigger asChild><span tabIndex={0} className="inline-flex">{children}</span></TooltipTrigger><TooltipContent className="max-w-[min(24rem,calc(100vw-2rem))]"><p className="mb-2 font-semibold break-words">{t("chainTitle", { name: host.name })}</p><TailnetChain host={host} compact /></TooltipContent></Tooltip>;
}

export function TailnetHostCause({ host }: { host: Host }) {
  const t = useTranslations("connections");
  const { status } = useTailnet();
  const [open, setOpen] = useState(false);
  if (host.networkRoute !== "tailnet") return null;
  const first = tailnetChain(host, status).find((row) => row.state === "failed");
  const offline = tailnetHostDown(host);
  return <>
    {host.tailnet?.tagged === false && <p role="status" className="text-sm text-warning">{t("tagMissing")}</p>}
    {offline && <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 p-3">
      <p className="min-w-0 text-sm">{first ? `${t(`links.${first.link}`)} · ${t("linkStates.failed")}` : t("noFailure")}</p>
      <Button variant="outline" size="sm" className="min-h-10" onClick={() => setOpen(true)}>{t("showCause")}</Button>
    </div>}
    <Dialog open={open} onOpenChange={setOpen}><DialogContent className="min-w-0 max-w-[calc(100vw-2rem)] sm:max-w-lg"><DialogHeader><DialogTitle className="break-words">{t("chainTitle", { name: host.name })}</DialogTitle><DialogDescription>{t("cause")}</DialogDescription></DialogHeader><TailnetChain host={host} />{first && <p className="text-sm text-muted-foreground">{t(`nextSteps.${first.link}`)}</p>}{first?.link === "hub" && <Button asChild variant="outline" className="min-h-10"><Link to="/settings/connections">{t("connectionsLink")}</Link></Button>}</DialogContent></Dialog>
  </>;
}
