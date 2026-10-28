import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { useTranslations } from "use-intl";

import { formatDate } from "@/domain/format";
import type { Finding, Host, ScannedImage, ScanRun } from "@/domain/model";
import { cn } from "@/shared/lib/utils";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/shared/ui/chart";

import { riskScore, SEVERITIES } from "./severity";

/** Semicircle risk gauge (Arcane security overview). */
export function RiskGauge({ images, top, onDriver }: { images: ScannedImage[]; top?: Finding; onDriver: (id: string) => void }) {
  const t = useTranslations("security.risk");
  const r = riskScore(images);
  return (
    <div className="flex flex-col items-center gap-2 text-center">
      <svg viewBox="0 0 200 118" className="w-full max-w-80" role="img" aria-label={t("label", { score: r.score, band: t(`bands.${r.band}`) })}>
        <defs>
          <linearGradient id="gauge-grad" x1="0" x2="1">
            <stop offset="0%" stopColor="var(--lime)" />
            <stop offset="50%" stopColor="var(--warning)" />
            <stop offset="100%" stopColor="var(--destructive)" />
          </linearGradient>
        </defs>
        <path d="M20 104 A80 80 0 0 1 180 104" pathLength={100} fill="none" stroke="var(--muted)" strokeWidth={14} strokeLinecap="round" />
        <path
          d="M20 104 A80 80 0 0 1 180 104"
          pathLength={100}
          fill="none"
          stroke="url(#gauge-grad)"
          strokeWidth={14}
          strokeLinecap="round"
          strokeDasharray={`${r.score} 100`}
          className="transition-[stroke-dasharray] duration-(--dur-slow) ease-(--ease-out)"
          style={{ filter: `drop-shadow(0 0 calc(8px * var(--glow)) ${r.color})` }}
        />
        <text x={100} y={90} textAnchor="middle" className="fill-foreground text-[34px] font-semibold tabular-nums">
          {r.score}
        </text>
        <text x={100} y={111} textAnchor="middle" className="text-[12px] font-medium" fill={r.color}>
          {t(`bands.${r.band}`)}
        </text>
      </svg>
      <p className="text-xs text-muted-foreground">{t("hint")}</p>
      {top && (
        <button type="button" onClick={() => onDriver(top.id)} className="lift rounded-lg px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted hover:text-foreground">
          {t("driver", { id: top.id, image: top.image.split("/").pop() ?? top.image })}
          <span className="block font-mono text-xs">
            {top.pkg} → {top.fixedIn ?? "–"}
          </span>
        </button>
      )}
    </div>
  );
}

const trendConfig = Object.fromEntries(SEVERITIES.map((s) => [s.key, { label: s.key, color: s.color }])) satisfies ChartConfig;

export function TrendChart({ history, animate, hosts }: { history: ScanRun[]; animate: boolean; hosts?: Map<string, Host> }) {
  const t = useTranslations("security");
  const data = history.toSorted((a, b) => Date.parse(a.at) - Date.parse(b.at)).filter((h) => h.sampled !== false).map((h) => ({ when: `${hosts?.get(h.hostId ?? "")?.name ?? h.hostId ?? ""} ${formatDate(h.at)} ${h.status ? t(`historyStatus.${h.status}`) : ""}`, manual: h.trigger === "manual", ...h.counts }));
  const config = Object.fromEntries(SEVERITIES.map((s) => [s.key, { label: t(`severities.${s.key}`), color: s.color }])) satisfies ChartConfig;
  if (!data.length) return <p className="text-sm text-muted-foreground">{t("historyEmpty")}</p>;
  return (
    <ChartContainer config={{ ...trendConfig, ...config }} className="aspect-auto h-44 w-full" aria-label={t("trend.label")}>
      <BarChart data={data} margin={{ left: -20, right: 0, top: 10 }}>
        <CartesianGrid vertical={false} strokeDasharray="3 3" />
        <XAxis
          dataKey="when"
          tickLine={false}
          axisLine={false}
          fontSize={10}
          tick={({ x, y, payload, index }) => (
            <text x={x} y={Number(y) + 10} textAnchor="middle" className="fill-muted-foreground text-[10px]">
              {payload.value}
              {data[index]?.manual ? " ▲" : ""}
            </text>
          )}
        />
        <YAxis tickLine={false} axisLine={false} fontSize={10} />
        <ChartTooltip content={<ChartTooltipContent />} />
        {[...SEVERITIES].reverse().map((s, i) => (
          <Bar key={s.key} dataKey={s.key} stackId="a" fill={`var(--color-${s.key})`} radius={i === SEVERITIES.length - 1 ? [3, 3, 0, 0] : 0} isAnimationActive={animate} animationDuration={600} />
        ))}
      </BarChart>
    </ChartContainer>
  );
}

export function ShareBar({ parts, className }: { parts: { key: string; label: string; color: string; n: number }[]; className?: string }) {
  return (
    <div className={className}>
      <div className="flex h-2 gap-0.5" aria-hidden>
        {parts.map((p) => (p.n ? <span key={p.key} className="min-w-1 rounded-full transition-[flex-grow] duration-(--dur-slow)" style={{ flexGrow: p.n, background: p.color }} title={`${p.label}: ${p.n}`} /> : null))}
      </div>
      <ul className="mt-2.5 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
        {parts.map((p) => (
          <li key={p.key} className="flex items-center gap-1.5">
            <i className="size-2 rounded-full" style={{ background: p.color }} aria-hidden />
            {p.label} <b className="font-medium text-foreground tabular">{p.n}</b>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function SevDot({ color, className }: { color: string; className?: string }) {
  return <i aria-hidden className={cn("inline-block size-2 shrink-0 rounded-full", className)} style={{ background: color }} />;
}
