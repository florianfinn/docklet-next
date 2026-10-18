// Source: docklet-next/issues/85
// Revision: 37ddf907edabe127fe061523c7aba1c764d87d85
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original acceptance correction and regression coverage.

import { useTranslations } from "use-intl";
import { MemoryStick } from "lucide-react";
import { formatMem } from "@/domain/format";
import { Meter } from "./meter";

export function ContainerMemory({ usedMb, limitMb, color }: { usedMb?: number; limitMb: number; color?: string }) {
  const t = useTranslations("common.load");
  if (limitMb <= 0) return <div className="text-xs"><span className="inline-flex items-center gap-1 text-muted-foreground"><MemoryStick className="size-3" />{t("ram")}</span><p>{usedMb === undefined ? "–" : formatMem(usedMb)}</p></div>;
  return <Meter label={t("ram")} icon={<MemoryStick />} value={usedMb === undefined ? 0 : usedMb / limitMb * 100} color={color} sub={usedMb === undefined ? undefined : `${formatMem(usedMb)} / ${formatMem(limitMb)}`} />;
}
