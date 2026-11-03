// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original fleet ring condition feed for issue #371.

import { useTranslations } from "use-intl";

import { useEvents } from "@/data/queries";
import { formatRelative } from "@/domain/format";
import type { EventLevel } from "@/domain/model";
import { ToneDot } from "@/shared/components/tone";

const TONE: Record<EventLevel, "err" | "warn" | "ok" | "info"> = { error: "err", warn: "warn", ok: "ok", info: "info" };

/** The last five container condition changes under the fleet ring; nothing while none exist. */
export function ConditionFeed() {
  const t = useTranslations("dashboard.ring");
  const events = useEvents();
  const recent = (events.data ?? []).filter((e) => e.condition).slice(0, 5);
  if (!recent.length) return null;
  return (
    <ol className="flex w-full max-w-md flex-col gap-1 text-xs" aria-label={t("feed")} data-slot="condition-feed">
      {recent.map((e) => (
        <li key={e.id} className="flex items-center gap-2">
          <ToneDot tone={TONE[e.level]} />
          <span className="min-w-0 flex-1 truncate">{e.title}</span>
          <span className="shrink-0 text-muted-foreground">{formatRelative(e.at)}</span>
        </li>
      ))}
    </ol>
  );
}
