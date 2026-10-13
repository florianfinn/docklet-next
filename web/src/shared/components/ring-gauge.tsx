import { cn } from "@/shared/lib/utils";

type GaugeLevel = "ok" | "warn" | "hot";

/** Same thresholds as the Arcane meter: warning from 75 %, critical from 90 %. */
function gaugeLevel(percent: number): GaugeLevel {
  if (percent >= 90) return "hot";
  if (percent >= 75) return "warn";
  return "ok";
}

const STROKE: Record<GaugeLevel, string> = {
  ok: "var(--gauge-color, var(--primary-text))",
  warn: "var(--warning)",
  hot: "var(--destructive)",
};

interface RingGaugeProps {
  /** 0–100; values above are clamped for the arc, `centre` shows the real number. */
  percent: number | null;
  centre: string;
  size?: number;
  color?: string;
  className?: string;
}

/**
 * Small circular gauge. `null` renders the unavailable state (dashed track,
 * dash in the centre) at the same size, so swapping states never shifts layout.
 */
export function RingGauge({ percent, centre, size = 30, color, className }: RingGaugeProps) {
  const stroke = 3.25;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const v = percent === null ? 0 : Math.max(0, Math.min(100, percent));
  const level = gaugeLevel(v);
  return (
    <span
      aria-hidden
      className={cn("relative inline-grid shrink-0 place-items-center", className)}
      style={{ width: size, height: size, "--gauge-color": color } as React.CSSProperties}
    >
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--muted)" strokeWidth={stroke} strokeDasharray={percent === null ? "2.5 3" : undefined} opacity={percent === null ? 0.9 : 0.7} />
        {percent !== null && (
          <circle
            className="ring-arc"
            cx={size / 2}
            cy={size / 2}
            r={r}
            fill="none"
            stroke={STROKE[level]}
            strokeWidth={stroke}
            strokeLinecap="round"
            strokeDasharray={c}
            strokeDashoffset={c * (1 - v / 100)}
          />
        )}
      </svg>
      <span className={cn("absolute font-mono text-[9.5px] leading-none font-semibold tabular", percent === null ? "text-muted-foreground" : level === "hot" ? "text-destructive" : level === "warn" ? "text-warning" : "text-foreground")}>
        {centre}
      </span>
    </span>
  );
}
