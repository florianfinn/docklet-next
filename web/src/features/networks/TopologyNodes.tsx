import { Handle, Position, type NodeProps } from "@xyflow/react";
import { ChevronDown, Network } from "lucide-react";
import { memo } from "react";
import { useTranslations } from "use-intl";

import { RUNTIME_TONE } from "@/domain/status";
import { cn } from "@/shared/lib/utils";

import { networkTone } from "./network-tone";
import { LINE, networkNameLines, networkSize } from "./topology-geometry";
import type { ContainerData, FocusAddress, LaneData, NetworkData, TopoNode } from "./topology-layout";

const hidden = "!size-1.5 !min-w-0 !border-0 !bg-transparent";

/** Group heading of one lane; not focusable or selectable. */
export const LaneNode = memo(function LaneNode({ data }: NodeProps<Extract<TopoNode, { type: "lane" }>>) {
  const t = useTranslations("networks.groups");
  const { group, count, width } = data as LaneData;
  return (
    <div style={{ width }} className="flex items-center gap-2 border-b whitespace-nowrap border-border/60 pb-1 text-[11px] font-semibold tracking-[0.06em] text-muted-foreground uppercase">
      {t(group)}
      <span className="font-normal tracking-normal normal-case">· {group === "isolated" ? t("containerCount", { count }) : t("count", { count })}</span>
    </div>
  );
});

/** Aggregated network: driver, address blocks of both families, member and problem counts; its toggle expands the containers. */
export const NetworkNode = memo(function NetworkNode({ data, selected }: NodeProps<Extract<TopoNode, { type: "network" }>>) {
  const t = useTranslations("networks");
  const { network, members, problems, expanded, dim, width } = data as NetworkData;
  const first = network.subnet ?? network.subnet6;
  // Same line count the layout reserved, so the block is exactly as tall as its slot.
  const nameLines = networkNameLines(network, networkSize(network).width);
  return (
    <div
      data-tone={networkTone(network)}
      style={{ width }}
      className={cn(
        "rounded-2xl border border-(--tone)/55 bg-(--tone)/12 px-[12px] py-[8px] shadow-md transition-opacity duration-(--dur)",
        selected && "ring-2 ring-(--tone)",
        dim && "opacity-25",
      )}
    >
      <Handle type="source" position={Position.Left} id="m" className={hidden} style={{ top: "calc(100% - 17px)" }} />
      <div className="flex items-start gap-[6px] text-[12.5px] leading-[18px] font-semibold text-foreground" style={{ height: nameLines * LINE.title }} data-slot="network-title">
        <Network className="mt-[2px] size-[14px] shrink-0 text-(--tone)" aria-hidden />
        {nameLines === 1 ? (
          <span className="truncate" title={network.name}>
            {network.name}
          </span>
        ) : (
          <span className="min-w-0 overflow-hidden break-all" style={{ display: "-webkit-box", WebkitBoxOrient: "vertical", WebkitLineClamp: nameLines }} title={network.name} data-name-lines={nameLines}>
            {network.name}
          </span>
        )}
        <span className="ml-auto flex h-[18px] shrink-0 items-center gap-[6px]">
          {network.internal && <span className="rounded bg-(--tone)/20 px-1 text-[9.5px] leading-normal text-(--tone)">{t("internal")}</span>}
          {network.builtIn && !network.internal && <span className="rounded bg-muted px-1 text-[9.5px] leading-normal text-muted-foreground">{t("standardShort")}</span>}
          {network.ipv6 && <span className="rounded bg-(--tone)/20 px-1 text-[9.5px] leading-normal text-(--tone)">{network.ipv4 === false ? "IPv6" : "v4/v6"}</span>}
        </span>
      </div>
      {/* One line per known address family; IPv6 never shares a line with IPv4, so neither is cut. */}
      <div className="mt-[2px] font-mono text-[10px] leading-[15px] text-muted-foreground" data-slot="network-subnets">
        <div className="flex justify-between gap-2">
          <span className="shrink-0">{network.parent ? `${network.driver} · ${network.parent}` : network.driver}</span>
          <span className="truncate" title={first}>
            {first ?? t("noSubnet")}
          </span>
        </div>
        {network.subnet && network.subnet6 && (
          <div className="truncate text-right" title={network.subnet6}>
            {network.subnet6}
          </div>
        )}
      </div>
      <div className="mt-[6px] flex h-[18px] items-center gap-1.5 text-[11px]">
        <span className="text-muted-foreground">{t("members", { count: members })}</span>
        {problems > 0 && (
          <span data-tone="err" className="rounded bg-(--tone)/15 px-1 text-(--tone)">
            {t("problemsShort", { count: problems })}
          </span>
        )}
        {members > 0 && (
          <span data-expand={network.id} className="nodrag ml-auto inline-flex items-center gap-0.5 rounded-md px-1 text-muted-foreground" aria-hidden>
            <ChevronDown className={cn("size-3 transition-transform", !expanded && "-rotate-90")} />
          </span>
        )}
      </div>
    </div>
  );
});

/** Focus addresses on the container block: one row per network, one line per known address. */
function AddressLines({ addresses }: { addresses: FocusAddress[] }) {
  const t = useTranslations("networks.detail");
  return (
    <ul className="mt-[2px] flex flex-col" aria-label={t("addresses")} data-slot="focus-addresses">
      {addresses.map((a) => (
        <li key={a.networkId} className="flex items-start gap-[6px] text-[10px] leading-[16px]">
          <span className="min-w-0 truncate text-muted-foreground" title={a.network}>
            {a.network}
          </span>
          {/* Addresses keep their full width; only the network name gives way at the widest column. */}
          <span className={cn("ml-auto flex shrink-0 flex-col items-end font-mono", a.kind === "ip" ? "text-cyan" : "text-muted-foreground")}>
            <span className="flex h-[16px] max-w-full items-center gap-1.5" title={a.kind === "host" ? t("hostAddressHint") : (a.address ?? undefined)}>
              <span className="truncate">{a.kind === "ip" ? a.address : a.kind === "host" ? (a.address ?? t("hostAddressUnknown")) : t("noAddress")}</span>
              {a.kind === "host" && a.address && <span className="shrink-0 rounded bg-muted px-1 font-sans text-[9px] text-muted-foreground">{t("hostAddressShort")}</span>}
            </span>
            {a.address6 && (
              <span className="h-[16px] max-w-full truncate" title={a.address6}>
                {a.address6}
              </span>
            )}
          </span>
        </li>
      ))}
    </ul>
  );
}

/** One container: status and origin only; live CPU/RAM stays in the details panel, so ticks never repaint the graph. */
export const ContainerNode = memo(function ContainerNode({ data, selected }: NodeProps<Extract<TopoNode, { type: "container" }>>) {
  const ts = useTranslations("common.status");
  const { container, host, status, dim, addresses, width } = data as ContainerData;
  const tone = RUNTIME_TONE[status];
  return (
    <div
      data-tone={tone}
      style={{ "--host": host?.color ?? "var(--border)", width } as React.CSSProperties}
      className={cn(
        "relative overflow-hidden rounded-xl border bg-card py-[8px] pr-[10px] pl-[14px] shadow-md transition-opacity duration-(--dur)",
        "before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-(--host)",
        status === "unhealthy" && "border-destructive/60",
        selected && "border-primary ring-2 ring-primary/60",
        dim && "opacity-25",
      )}
    >
      {/* Left middle: membership bus; right: dependency arrows out (r) and in (rt); upper left: arrows from the left column (l). */}
      <Handle type="target" position={Position.Left} id="m" className={hidden} style={{ top: 22 }} />
      <Handle type="source" position={Position.Right} id="r" className={hidden} style={{ top: 22 }} />
      <Handle type="target" position={Position.Right} id="rt" className={hidden} style={{ top: 40 }} />
      <Handle type="target" position={Position.Left} id="l" className={hidden} style={{ top: 8 }} />
      <div className="flex h-[18px] items-center gap-[6px]">
        <span className="size-[8px] shrink-0 rounded-full bg-(--tone)" aria-hidden />
        <span className="truncate text-[12.5px] font-semibold" title={container.serviceName ?? container.name}>
          {container.serviceName ?? container.name}
        </span>
      </div>
      <div className="mt-[2px] h-[15px] truncate text-[10.5px] leading-[15px] text-muted-foreground">
        {container.stackId ?? container.origin} · {ts(status)}
      </div>
      {addresses.length > 0 && <AddressLines addresses={addresses} />}
    </div>
  );
});
