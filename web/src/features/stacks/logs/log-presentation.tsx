// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original shared localized log delivery notices.

import { useTranslations } from "use-intl";
import type { LogLine } from "@/domain/model";
import { logDelivery, logGap } from "@/data/log-buffer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { Button } from "@/shared/ui/button";

export function LogDeliveryNotice({ lines, paused }: { lines: LogLine[]; paused: boolean }) {
  const t = useTranslations("stacks.logs.delivery");
  const delivery = logDelivery(lines);
  if (!delivery && !paused) return null;
  return <div role="status" className="flex flex-wrap items-center gap-2 border-b px-3 py-2 text-xs text-muted-foreground">
    <span>{paused ? t("paused") : delivery && t(delivery.status)}</span>
    {delivery?.reconnect && <span>{t("boundary")}</span>}
    {!!delivery?.dropped && <span>{t("dropped", { count: delivery.dropped })}</span>}
    {!paused && (delivery?.status === "error" || delivery?.status === "unavailable") && delivery.retry && <Button size="sm" variant="outline" onClick={delivery.retry}>{t("retry")}</Button>}
  </div>;
}
export function LogSource({ line }: { line: LogLine }) {
  const t = useTranslations("stacks.logs.delivery");
  if (logGap(line) !== undefined) return null;
  const stream = t(`stream.${line.stream ?? "unknown"}`);
  return <Tooltip>
    <TooltipTrigger asChild><span>{line.service || t("unknownSource")}<span className="sr-only">{` · ${stream}`}</span></span></TooltipTrigger>
    <TooltipContent>{stream}</TooltipContent>
  </Tooltip>;
}
export function LogMessage({ line }: { line: LogLine }) {
  const t = useTranslations("stacks.logs.delivery");
  const gap = logGap(line);
  if (gap !== undefined) return <span>{t("loss", { count: gap })}</span>;
  return <>{line.message}{line.truncated && <span className="ml-2 text-warning">{t("lineCut")}</span>}</>;
}
