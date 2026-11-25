// Source: docklet-next/issues/636
// Revision: 7a44b4b5c32ccc64b68e027a5496fab1d26e90e2
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 stack Health tab and reusable health details.

import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";
import type { Host, Service, StackDetail } from "@/domain/model";
import { HISTORY_RANGES, type HistoryRange } from "@/domain/history";
import { RUNTIME_TONE, serviceRuntime } from "@/domain/status";
import { ToneBadge } from "@/shared/components/tone";
import { HealthDetails } from "@/shared/health/HealthDetails";
import { ToggleGroup, ToggleGroupItem } from "@/shared/ui/toggle-group";

const RANGE_KEY = { "24h": "r24h", "7d": "r7d", "30d": "r30d" } as const;

export function HealthTab({ stack, host }: { stack: StackDetail; host?: Host }) {
  const [range, setRange] = useState<HistoryRange>("24h");
  const [params] = useSearchParams();
  const t = useTranslations("history");
  return <div className="grid gap-4">
    <ToggleGroup type="single" variant="outline" value={range} aria-label={t("rangeLabel")} onValueChange={(v) => { if (v) setRange(v as HistoryRange); }}>
      {HISTORY_RANGES.map((r) => <ToggleGroupItem key={r} value={r}>{t(`ranges.${RANGE_KEY[r]}`)}</ToggleGroupItem>)}
    </ToggleGroup>
    {stack.services.map((service) => <ServiceHealth key={service.name} service={service} host={host} hostId={stack.hostId} range={range} focused={params.get("service") === service.name} />)}
  </div>;
}

function ServiceHealth({ service, host, hostId, range, focused }: { service: Service; host?: Host; hostId: string; range: HistoryRange; focused: boolean }) {
  const ref = useRef<HTMLElement>(null);
  const t = useTranslations("common.status");
  const status = serviceRuntime(service, host);
  useEffect(() => {
    if (focused) {
      ref.current?.scrollIntoView({ block: "center" });
      ref.current?.focus({ preventScroll: true });
    }
  }, [focused]);
  return <section ref={ref} tabIndex={-1} aria-label={service.name} className="grid min-w-0 scroll-mt-24 gap-3 rounded-xl border p-4 outline-none focus:ring-2 focus:ring-ring">
    <h3 className="flex items-center gap-3 font-medium">{service.name}<ToneBadge tone={RUNTIME_TONE[status]}>{t(status)}</ToneBadge></h3>
    <HealthDetails key={service.containerId ?? "none"} containerId={service.containerId} historyRef={service.containerName ? { hostId, name: service.containerName } : undefined} range={range} />
  </section>;
}
