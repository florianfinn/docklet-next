import { Cpu, HardDrive, MemoryStick, Unplug } from "lucide-react";
import { useTranslations } from "use-intl";

import { useMetrics } from "@/data/live";
import { formatGb, formatRelative } from "@/domain/format";
import { hostTone } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import { Sparkline } from "@/shared/component-set/sparkline";
import { Meter } from "@/shared/components/meter";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";

/** Arcane environment card: live CPU trend plus CPU/RAM/disk meters (simulated). */
export function HostPulse({ host, stacks, containers, className }: { host: Host; stacks: number; containers: number; className?: string }) {
  const t = useTranslations("dashboard.summary");
  const tc = useTranslations("common.host");
  const m = useMetrics((s) => s?.hosts[host.id]);
  const offline = host.status !== "online";
  const pending = host.status === "pending";
  return (
    <article
      data-glow
      style={{ "--host": host.color } as React.CSSProperties}
      className={cn(
        // Opaque card surface: the moving dot field behind the summary never shows through text or charts.
        "lift relative flex h-full min-w-0 flex-col gap-3 overflow-hidden rounded-xl border bg-[color-mix(in_oklch,var(--card),var(--foreground)_4%)] p-3.5 shadow-sm",
        "before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-(--host)",
        className,
      )}
    >
      <header className="flex items-center gap-2">
        <span className="size-2.5 rounded-[3px] bg-(--host)" aria-hidden />
        <h3 className="font-semibold tracking-tight">{host.name}</h3>
        <ToneBadge tone={host.connection === "edge" ? "info" : "accent"} className="h-5 px-1.5 text-[10.5px]">
          {host.connection === "edge" ? tc("edge") : tc("direct")}
        </ToneBadge>
        <span className="ml-auto flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <ToneDot tone={hostTone(host)} pulse={offline} />
          {tc(host.status)}
        </span>
      </header>
      {/* Grows when the card stretches to the host area's height; the trend takes the extra space. */}
      <div className="flex min-h-9 flex-1 items-end justify-between gap-3">
        <div className="shrink-0 text-[11px] leading-tight text-muted-foreground">
          <div>{host.os}</div>
          <div>
            {t("hostStacks", { count: stacks })} · {t("hostContainers", { count: containers })}
          </div>
        </div>
        {m && <Sparkline data={m.cpuHistory} type="area" color={offline ? "var(--muted-foreground)" : host.color} domain={[0, 100]} className="h-auto min-h-9 w-[45%] max-w-64 min-w-28 self-stretch" label={`CPU ${host.name}`} />}
      </div>
      {pending ? (
        <p className="flex items-center gap-1.5 text-xs text-info">
          <Unplug className="size-3.5" aria-hidden />
          {t("pending")}
        </p>
      ) : offline ? (
        <p data-tone="orange" className="flex items-center gap-1.5 text-xs text-(--tone-text)">
          <Unplug className="size-3.5" aria-hidden />
          {t("offline", { time: formatRelative(host.lastSeen) })} · {t("stale")}
        </p>
      ) : (
        <div className="grid grid-cols-3 gap-2.5">
          <Meter label="CPU" icon={<Cpu />} value={m?.cpu ?? 0} size="sm" color={host.color} sub={t("cores", { count: host.cpuCores })} />
          <Meter label="RAM" icon={<MemoryStick />} value={m?.mem ?? 0} size="sm" color={host.color} sub={t("of", { total: `${host.memTotalGb} GB` })} />
          <Meter label="Disk" icon={<HardDrive />} value={m?.disk ?? 0} size="sm" color={host.color} sub={t("of", { total: formatGb(host.diskTotalGb) })} />
        </div>
      )}
    </article>
  );
}
