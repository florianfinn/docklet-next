import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { ArrowDownToLine, Eraser, Pause, Play, Search, WrapText } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { logGap } from "@/data/log-buffer";
import { LogDeliveryNotice, LogMessage, LogSource } from "./log-presentation";
import { useStackLogs } from "@/data/live";
import { formatTime } from "@/domain/format";
import type { LogLevel, Stack } from "@/domain/model";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { cn } from "@/shared/lib/utils";
import { Card } from "@/shared/ui/card";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";
import { Toggle } from "@/shared/ui/toggle";

const SERVICE_TONES = ["info", "violet", "teal", "orange", "pink", "lime", "cyan"] as const;

type LevelFilter = "all" | Exclude<LogLevel, "debug">;

const LEVEL_CLASS: Record<LogLevel, string> = {
  info: "",
  debug: "text-muted-foreground",
  warn: "bg-warning/8 text-warning",
  error: "bg-destructive/10 text-destructive",
};

/** Shared multiplexed log stream of all stack services with origin per line. */
export function StackLogs({ stack }: { stack: Stack }) {
  return <StackLogsView key={stack.id} stack={stack} />;
}

function StackLogsView({ stack }: { stack: Stack }) {
  const t = useTranslations("stacks.logs");
  const [params] = useSearchParams();
  const initial = params.get("service");
  const [paused, setPaused] = useState(false);
  const lines = useStackLogs(stack.id, paused);
  const [services, setServices] = useState<string[]>(initial && stack.services.some((service) => service.name === initial) ? [initial] : []);
  const [level, setLevel] = useState<LevelFilter>("all");
  const [query, setQuery] = useState("");
  const [follow, setFollow] = useState(true);
  const [stamps, setStamps] = useState(true);
  const [wrap, setWrap] = useState(false);
  const [clearedIds, setClearedIds] = useState<Set<string>>(() => new Set());
  const scroller = useRef<HTMLDivElement>(null);

  const tones = useMemo(() => new Map(stack.services.map((s, i) => [s.name, SERVICE_TONES[i % SERVICE_TONES.length] ?? "info"])), [stack.services]);
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return lines.filter(
      (l) =>
        !clearedIds.has(l.id) && (logGap(l) !== undefined || (
        (services.length === 0 || services.includes(l.service)) &&
        (level === "all" || l.level === level) &&
        (!q || l.message.toLowerCase().includes(q) || l.service.toLowerCase().includes(q)))),
    );
  }, [lines, services, level, query, clearedIds]);

  useEffect(() => {
    if (follow && scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [shown, follow]);

  return (
    <Card className="gap-0 overflow-hidden py-0">
      <div className="flex flex-wrap items-center gap-2 border-b p-3">
        <InputGroup className="w-full sm:w-64">
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupInput type="search" placeholder={t("search")} aria-label={t("searchLabel")} value={query} onChange={(e) => setQuery(e.target.value)} />
        </InputGroup>
        <SegmentedToggle<LevelFilter> value={level} onChange={setLevel} label={t("level")} options={(["all", "info", "warn", "error"] as const).map((l) => ({ value: l, label: t(`levels.${l}`) }))} fit="content" wrap />
        <span className="flex-1" />
        <Toggle size="sm" variant="outline" pressed={follow} onPressedChange={setFollow} aria-label={t("follow")}>
          <ArrowDownToLine /> <span className="hidden md:inline">{t("follow")}</span>
        </Toggle>
        <Toggle size="sm" variant="outline" pressed={stamps} onPressedChange={setStamps} aria-label={t("timestamps")}>
          <span className="font-mono text-[11px]">12:00</span>
        </Toggle>
        <Toggle size="sm" variant="outline" pressed={wrap} onPressedChange={setWrap} aria-label={t("wrap")}>
          <WrapText />
        </Toggle>
        <Toggle size="sm" variant="outline" pressed={paused} onPressedChange={setPaused} aria-label={paused ? t("resume") : t("pause")}>
          {paused ? <Play /> : <Pause />}
        </Toggle>
        <Toggle size="sm" variant="outline" pressed={false} onPressedChange={() => setClearedIds(new Set(lines.map((line) => line.id)))} aria-label={t("clear")}>
          <Eraser />
        </Toggle>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 border-b px-3 py-2" role="group" aria-label={t("services")}>
        {stack.services.map((s) => {
          const tone = tones.get(s.name) ?? "info";
          const on = services.length === 0 || services.includes(s.name);
          return (
            <button
              key={s.name}
              type="button"
              aria-pressed={services.includes(s.name)}
              data-tone={tone}
              onClick={() => setServices((cur) => (cur.includes(s.name) ? cur.filter((x) => x !== s.name) : [...cur, s.name]))}
              className={cn(
                "inline-flex h-6 items-center gap-1.5 rounded-md border px-2 font-mono text-[11px] transition-[opacity,background] duration-(--dur-fast) active:scale-95",
                on ? "border-(--tone)/40 bg-(--tone)/12 text-(--tone)" : "border-dashed text-muted-foreground opacity-60",
              )}
            >
              <i className="size-1.5 rounded-full bg-(--tone)" aria-hidden />
              {s.name}
            </button>
          );
        })}
        <span className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
          {paused && <span className="text-warning">{t("paused")}</span>}
          {t("lines", { shown: shown.length, total: lines.length })}
          <SimulationBadge />
        </span>
      </div>
      <LogDeliveryNotice lines={lines} paused={paused} />
      <div ref={scroller} role="log" tabIndex={0} aria-live={paused ? "off" : "polite"} aria-label={t("title")} className="h-[max(24rem,calc(100dvh-24rem))] overflow-auto bg-code py-2 font-mono text-[12px] leading-relaxed">
        {shown.length === 0 && <p className="px-4 py-6 text-center text-muted-foreground">{t("empty")}</p>}
        {shown.map((l) => {
          const tone = tones.get(l.service) ?? "info";
          return (
            <div key={l.id} className={cn("flex gap-3 px-3 hover:bg-muted/30", LEVEL_CLASS[l.level], wrap ? "whitespace-pre-wrap" : "whitespace-pre")}>
              {stamps && <span className="shrink-0 text-muted-foreground/70 select-none">{formatTime(l.ts, true)}</span>}
              <span data-tone={tone} className="w-28 shrink-0 truncate text-(--tone) select-none">
                <LogSource line={l} />
              </span>
              <span className="min-w-0"><LogMessage line={l} /></span>
            </div>
          );
        })}
      </div>
    </Card>
  );
}
