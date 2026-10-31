import { hostTone } from "@/domain/hosts";
import { ArrowRight, Boxes, Layers, MousePointerClick, Unplug } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatRelative } from "@/domain/format";
import type { Host } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE } from "@/domain/status";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";

import { hrefOf } from "./fleet-segments";
import { FLEET_TONE, lastKnownState, type FleetUnit } from "./fleet-status";
import { useUnitName } from "./unit-name";

const MEMBERS = 6;

/**
 * Rich details of one stack or group segment, styled like the update hover
 * cards. `info` is the read-only card beside the ring: no controls, a hint to
 * click or open the actions menu instead of an open button.
 */
export function SegmentCard({ unit: u, host, variant = "popover" }: { unit: FleetUnit; host: Host | undefined; variant?: "popover" | "info" }) {
  const t = useTranslations("dashboard.ring");
  const tv = useTranslations("visual.ring");
  const ts = useTranslations("common.status");
  const name = useUnitName();
  const tone = FLEET_TONE[u.state];
  const unreachable = u.state === "unreachable";
  return (
    <div className="text-xs">
      <div data-tone={tone} className="tone-surface flex items-start gap-3 p-3.5">
        <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-(--tone)/15 text-(--tone) [&_svg]:size-4">{unreachable ? <Unplug /> : u.kind === "stack" ? <Layers /> : <Boxes />}</span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold text-foreground">{name(u)}</div>
          <div className="truncate text-muted-foreground">{u.kind === "stack" ? t("stackSummary", { running: u.running, total: u.containers.length }) : t("groupSummary", { running: u.running, total: u.containers.length })}</div>
        </div>
        <ToneBadge tone={tone} className="h-5 shrink-0 px-1.5 text-[10.5px]">
          {unreachable ? t("unreachable") : ts(u.state)}
        </ToneBadge>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 px-3.5 pt-3">
        <dt className="text-muted-foreground">{t("host")}</dt>
        <dd className="flex min-w-0 items-center gap-1.5">
          <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: host?.color }} aria-hidden />
          <span className="truncate font-medium">{host?.name ?? u.hostId}</span>
          {host && host.status !== "online" && <ToneDot tone={hostTone(host)} pulse className="size-1.5" />}
        </dd>
        {unreachable && (
          <>
            <dt className="text-muted-foreground">{t("lastKnown")}</dt>
            <dd>{t("lastKnownTime", { time: host ? formatRelative(host.lastSeen) : "–" })}</dd>
          </>
        )}
      </dl>
      <ul className="flex flex-col gap-1 px-3.5 py-2.5" aria-label={t("members")} data-slot="segment-members">
        {u.containers.slice(0, MEMBERS).map((c) => {
          const st = unreachable ? lastKnownState(c) : containerRuntime(c, host);
          return (
            <li key={c.id} className="flex min-w-0 items-center gap-2">
              <ToneDot tone={RUNTIME_TONE[st]} className="size-1.5" />
              <span className="min-w-0 flex-1 truncate">{c.serviceName ?? c.name}</span>
              <span className="shrink-0 text-muted-foreground">{unreachable ? t("lastKnownState", { state: ts(st) }) : ts(st)}</span>
            </li>
          );
        })}
        {u.containers.length > MEMBERS && <li className="text-muted-foreground">{t("moreMembers", { count: u.containers.length - MEMBERS })}</li>}
      </ul>
      {unreachable && (
        <p data-tone="err" className="mx-3.5 mb-3 rounded-lg bg-(--tone)/10 p-2.5 leading-relaxed text-(--tone)">
          {t("unreachableNote", { host: host?.name ?? u.hostId })}
        </p>
      )}
      {variant === "info" ? (
        <p className="flex items-center gap-2 border-t border-border/50 bg-muted/40 px-3.5 py-2.5 text-muted-foreground" data-slot="segment-hint">
          <MousePointerClick className="size-3.5 shrink-0" aria-hidden />
          {tv("infoHint")}
        </p>
      ) : (
        <div className="border-t border-border/50 bg-muted/40 p-2.5">
          <Button asChild size="sm" variant="secondary" className="group w-full">
            <Link to={hrefOf(u)}>
              {u.kind === "stack" ? t("openStack") : u.containers.length === 1 ? t("openContainer") : t("openGroup")}
              <ArrowRight data-icon="inline-end" className="transition-transform group-hover:translate-x-0.5" />
            </Link>
          </Button>
        </div>
      )}
    </div>
  );
}
