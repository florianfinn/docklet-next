import { useTranslations } from "use-intl";

import { useMetrics } from "@/data/live";
import { formatMem } from "@/domain/format";
import { formatNumber, formatPercentNumber } from "@/domain/locale";
import { cn } from "@/shared/lib/utils";
import { useUiStyle, type MetricDisplay } from "@/shared/preferences/ui-style-store";

import { MiniBar } from "./meter";
import { RingGauge } from "./ring-gauge";

interface LoadValues {
  cpu: number;
  memMb: number;
}

interface ContainerLoadProps {
  /** Container id in the metrics snapshot. */
  id: string;
  memLimitMb: number;
  /** Stopped container or disconnected host: no live values. */
  unavailable: boolean;
  /** Overrides the design-lab choice, e.g. for previews. */
  display?: MetricDisplay;
  className?: string;
}

/** Simulated CPU/RAM of one container as rings or slim bar; fixed width either way. */
export function ContainerLoad({ id, memLimitMb, unavailable, display, className }: ContainerLoadProps) {
  const m = useMetrics((s) => s?.containers[id]);
  const values = unavailable || !m || m.memMb === 0 ? null : m;
  return <LoadView values={values} memLimitMb={memLimitMb} display={display} className={className} />;
}

export function LoadView({ values, memLimitMb, display, className }: { values: LoadValues | null; memLimitMb: number; display?: MetricDisplay; className?: string }) {
  const t = useTranslations("common.load");
  const chosen = useUiStyle("metricDisplay");
  const mode = display ?? chosen;

  if (mode === "bars") {
    if (!values) return <span className={cn("inline-block w-[10.5rem] text-muted-foreground", className)}>–</span>;
    return (
      <span className={cn("flex w-[10.5rem] items-center gap-2 font-mono text-xs tabular", className)}>
        <MiniBar value={values.cpu} />
        {formatPercentNumber(values.cpu)}<span className="text-muted-foreground">· {formatMem(values.memMb)}</span>
      </span>
    );
  }

  const memPct = values ? (values.memMb / Math.max(1, memLimitMb)) * 100 : null;
  return (
    <span data-slot="container-load" data-display="rings" className={cn("flex w-[10.5rem] items-center gap-2", className)}>
      <RingMetric
        label={t("cpu")}
        percent={values ? values.cpu : null}
        centre={values ? String(Math.round(values.cpu)) : "–"}
        detail={values ? formatPercentNumber(values.cpu) : t("off")}
        valueText={values ? t("cpuValue", { value: formatNumber(values.cpu) }) : t("unavailable")}
      />
      <RingMetric
        label={t("ram")}
        percent={memPct}
        centre={memPct === null ? "–" : String(Math.round(memPct))}
        detail={values ? formatMem(values.memMb) : t("off")}
        valueText={values && memPct !== null ? t("ramValue", { used: formatMem(values.memMb), limit: formatMem(memLimitMb), percent: Math.round(memPct) }) : t("unavailable")}
      />
    </span>
  );
}

function RingMetric({ label, percent, centre, detail, valueText }: { label: string; percent: number | null; centre: string; detail: string; valueText: string }) {
  const meter =
    percent === null
      ? { role: "img", "aria-label": `${label}: ${valueText}` }
      : { role: "meter", "aria-label": label, "aria-valuenow": Math.round(Math.min(100, percent)), "aria-valuemin": 0, "aria-valuemax": 100, "aria-valuetext": valueText };
  return (
    <span {...meter} title={`${label}: ${valueText}`} className="flex min-w-0 flex-1 items-center gap-1">
      <RingGauge percent={percent} centre={centre} size={27} />
      <span className="grid min-w-0 leading-tight">
        <span className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">{label}</span>
        <span className={cn("font-mono text-[11px] whitespace-nowrap tabular", percent === null && "text-muted-foreground")}>{detail}</span>
      </span>
    </span>
  );
}
