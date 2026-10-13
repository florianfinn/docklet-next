import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { useMemo, useState } from "react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { useTranslations } from "use-intl";

import { useMetrics } from "@/data/live";
import type { Host, MetricsSnapshot } from "@/domain/model";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/shared/ui/chart";

/** Live CPU/RAM history per visible host; values come from the simulator. */
export function LoadWidget({ hosts }: { hosts: Host[] }) {
  const t = useTranslations("dashboard.load");
  const [metric, setMetric] = useState<"cpu" | "mem">("cpu");
  const snapshot = useMetrics((s) => s) as MetricsSnapshot | undefined;
  const online = hosts.filter((h) => h.status === "online");
  const config = useMemo(() => Object.fromEntries(online.map((h) => [h.id, { label: h.name, color: h.color }])) satisfies ChartConfig, [online]);
  const data = useMemo(() => {
    const len = Math.max(0, ...online.map((h) => snapshot?.hosts[h.id]?.cpuHistory.length ?? 0));
    return Array.from({ length: len }, (_, i) => {
      const row: Record<string, number> = { i };
      for (const h of online) {
        const hm = snapshot?.hosts[h.id];
        row[h.id] = (metric === "cpu" ? hm?.cpuHistory : hm?.memHistory)?.[i] ?? 0;
      }
      return row;
    });
  }, [snapshot, online, metric]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <SegmentedToggle<"cpu" | "mem">
          value={metric}
          onChange={setMetric}
          label={t("toggle")}
          options={[
            { value: "cpu", label: t("cpu") },
            { value: "mem", label: t("mem") },
          ]}
        />
        <SimulationBadge />
      </div>
      <ChartContainer config={config} className="aspect-auto min-h-28 w-full flex-1">
        <AreaChart data={data} margin={{ left: -18, right: 4, top: 8, bottom: 0 }}>
          <defs>
            {online.map((h) => (
              <linearGradient key={h.id} id={`load-${h.id}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={`var(--color-${h.id})`} stopOpacity={0.35} />
                <stop offset="100%" stopColor={`var(--color-${h.id})`} stopOpacity={0} />
              </linearGradient>
            ))}
          </defs>
          <CartesianGrid vertical={false} strokeDasharray="3 3" />
          <XAxis dataKey="i" hide />
          <YAxis domain={[0, 100]} tickCount={5} tickLine={false} axisLine={false} tickFormatter={(v: number) => t("axis", { value: v })} fontSize={10} />
          <ChartTooltip content={<ChartTooltipContent indicator="line" labelFormatter={() => ""} />} />
          {online.map((h) => (
            <Area key={h.id} dataKey={h.id} type="monotone" stroke={`var(--color-${h.id})`} strokeWidth={2} fill={`url(#load-${h.id})`} isAnimationActive={false} />
          ))}
        </AreaChart>
      </ChartContainer>
    </div>
  );
}
