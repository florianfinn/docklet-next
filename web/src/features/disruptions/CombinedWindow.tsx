// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original disruption timeline and bounded combined log window for #693.

import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import type { LogReadResult } from "@/data/api";
import { logLifetime } from "@/data/log-buffer";
import type { LogLine } from "@/domain/model";
import { LogDensity, ErrorJump } from "@/shared/logs/LogInsights";
import { Button } from "@/shared/ui/button";
import type { WindowSelection } from "./participants";
interface Read { lines: LogLine[]; result?: LogReadResult }
// Up to 20 participants × 2,000 lines keeps the merged, rendered window within 40,000 rows.
const PARTICIPANT_LINES = 2000;
export function CombinedWindow({ selection }: { selection: WindowSelection }) {
  const t = useTranslations("disruptions"), adapter = useAdapter();
  const lifetime = logLifetime(adapter.subscribeLogs);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const [attempt, setAttempt] = useState(0);
  const [view, setView] = useState<{ selection: WindowSelection; adapter: typeof adapter; generation: number; attempt: number; reads: Record<string, Read> }>();
  const [focused, setFocused] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    const update = (id: string, lines?: LogLine[], result?: LogReadResult) => {
      if (controller.signal.aborted) return;
      setView((old) => {
        const reads = old?.selection === selection && old.adapter === adapter && old.generation === generation && old.attempt === attempt ? old.reads : {};
        return { selection, adapter, generation, attempt, reads: { ...reads, [id]: { lines: [...(reads[id]?.lines ?? []), ...(lines ?? [])], result } } };
      });
    };
    for (const p of selection.participants) if (p.id) {
      const id = p.id;
      void adapter.readLogs({ kind: "container", id }, { since: selection.since, until: selection.until, tail: "all", maxLines: PARTICIPANT_LINES, maxBytes: 1024 * 1024 }, (lines) => update(id, lines), controller.signal).then((result) => update(id, undefined, result)).catch(() => update(id, undefined, { reason: "unavailable", lines: 0, bytes: 0 }));
    }
    return () => controller.abort();
  }, [selection, adapter, generation, attempt]);
  const reads = view?.selection === selection && view.adapter === adapter && view.generation === generation && view.attempt === attempt ? view.reads : {};
  const lines = useMemo(() => selection.participants.flatMap((p) => (p.id ? reads[p.id]?.lines ?? [] : []).map((line, i) => ({ ...line, id: `${p.id}:${i}`, service: `${p.name} · ${p.host} [${p.id}]` }))).sort((a, b) => Date.parse(a.ts) - Date.parse(b.ts)), [selection, reads]);
  const focus = (id: string) => { setFocused(id); document.getElementById(`disruption-log-${id}`)?.scrollIntoView?.({ block: "nearest" }); };
  const jump = (direction: -1 | 1) => {
    const marked = lines.filter((l) => l.level === "error" || l.level === "warn");
    const index = marked.findIndex((l) => l.id === focused);
    const next = index < 0 ? marked.at(direction === 1 ? 0 : -1) : marked[(index + direction + marked.length) % marked.length];
    if (next) focus(next.id);
  };
  return <section className="grid min-w-0 gap-3 rounded-xl border p-4" aria-label={t("combined")}>
    <h2 className="text-lg font-semibold">{t("combined")}</h2><p>{selection.since} – {selection.until}</p>
    {selection.omitted > 0 && <p role="status">{t("omitted", { count: selection.omitted })}</p>}
    <ul>{selection.participants.map((p) => {
      const read = p.id ? reads[p.id] : undefined;
      return <li key={`${p.hostId}:${p.name}`}><span>{p.name} · {p.host}{p.id ? ` [${p.id}]` : ""}</span>{!p.id ? <p>{t("missing")}</p> : read?.result?.reason === "unavailable" ? <p role="status">{t("unavailable")}</p> : read?.result ? <>{read.lines.length === 0 && <p>{t("emptyLogs")}</p>}{(read.result.reason === "cap-lines" || read.result.reason === "cap-bytes") && <p>{t("capped")}</p>}</> : <p role="status">{t("loading")}</p>}</li>;
    })}</ul>
    <div className="flex flex-wrap gap-2"><ErrorJump jump={jump} /><Button variant="outline" onClick={() => setAttempt((n) => n + 1)}>{t("retry")}</Button></div>
    <LogDensity lines={lines} onSelect={focus} />
    <ol className="max-h-96 overflow-auto font-mono text-xs">{lines.map((l) => <li key={l.id} id={`disruption-log-${l.id}`} className={focused === l.id ? "bg-muted" : ""}><time>{l.ts}</time> [{l.service}] {l.message}</li>)}</ol>
  </section>;
}
