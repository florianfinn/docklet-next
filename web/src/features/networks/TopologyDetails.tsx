import { isHostAvailable } from "@/domain/hosts";
import { useCapability } from "@/data/use-capability";
import { useNetworkCapabilities } from "./use-network-capabilities";
import { ArrowRight, Box, ChevronsDownUp, ChevronsUpDown, Link2, Network, Trash2, Unlink, X } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useMetrics } from "@/data/live";
import { formatMem, formatPorts } from "@/domain/format";
import { formatPercentNumber } from "@/domain/locale";
import type { ContainerInfo, Host, Topology, TopologyNetwork } from "@/domain/model";
import { networkComposeOwner, removeBlock } from "@/domain/networks";
import { containerRuntime, RUNTIME_TONE } from "@/domain/status";
import { versionLabel } from "@/domain/updates";
import { HostChip } from "@/shared/components/host-chip";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import type { ConnectRequest } from "./ConnectNetworkDialog";
import { hostAddress } from "./host-address";
import { networkTone } from "./network-tone";

interface DetailsProps {
  selected: string | null;
  topology: Topology;
  containers: ContainerInfo[];
  hosts: Map<string, Host>;
  onSelect: (id: string | null) => void;
  expanded: ReadonlySet<string>;
  onToggleExpand: (networkId: string) => void;
  onConnect: (request: ConnectRequest) => void;
  onRemove: (network: TopologyNetwork) => void;
}

function Kv({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}

/** Selection details: identity, addresses, memberships and configured dependencies, with network actions. */
export function TopologyDetails({ selected, topology, containers, hosts, onSelect, expanded, onToggleExpand, onConnect, onRemove }: DetailsProps) {
  const t = useTranslations("networks.detail");
  const tn = useTranslations("networks");
  const ts = useTranslations("common.status");
  const metrics = useMetrics((s) => (selected ? s?.containers[selected] : undefined));
  const net = topology.networks.find((n) => n.id === selected);
  const container = containers.find((c) => c.id === selected);

  const selectedNetworks = topology.networks.filter((n) => net ? n.id === net.id : n.hostId === container?.hostId).map((n) => n.id);
  const connect = useNetworkCapabilities("connectNetwork", selectedNetworks);
  const disconnect = useNetworkCapabilities("disconnectNetwork", selectedNetworks);
  const remove = useCapability("removeNetwork", { kind: "network", id: net?.id ?? "" });
  const connectAvailable = topology.networks.some((n) => selectedNetworks.includes(n.id) && !n.detailUnavailable && !n.inventory?.stale && connect.get(n.id)?.available);

  if (!net && !container) {
    return (
      // As tall as its hint: the network list below stays in view.
      <Card className="flex-row items-center gap-3 p-4" data-slot="topology-details-empty">
        <IconTile tone="cyan">
          <Network />
        </IconTile>
        <p className="min-w-0 text-sm text-muted-foreground">{t("empty")}</p>
      </Card>
    );
  }
  const containerName = (id: string) => {
    const c = containers.find((x) => x.id === id);
    return c?.serviceName ?? c?.name ?? id;
  };
  const name = (id: string) => topology.networks.find((n) => n.id === id)?.name ?? containerName(id);
  const host = hosts.get(net?.hostId ?? container?.hostId ?? "");
  const online = isHostAvailable(host);
  const hostAddr = hostAddress(host);
  const peers = topology.attachments.filter((a) => (net ? a.networkId === net.id : a.containerId === container?.id));
  const dependsOn = container ? topology.dependencies.filter((d) => d.from === container.id).map((d) => d.to) : [];
  const requiredBy = container ? topology.dependencies.filter((d) => d.to === container.id).map((d) => d.from) : [];
  const rt = container ? containerRuntime(container, host) : "running";
  const blocked = net ? removeBlock(topology, net) : undefined;
  const lan = net?.driver === "macvlan" || net?.driver === "ipvlan";

  const removeButton = net && (
    <Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" disabled={!remove.available || !!net?.inventory?.stale || !!blocked || !online} title={blocked ? tn(`removeBlocked.${blocked}`, { stack: net ? networkComposeOwner(net) ?? "" : "", count: peers.length }) : remove.reason} onClick={() => onRemove(net)}>
      <Trash2 data-icon="inline-start" />
      {t("remove")}
    </Button>
  );

  return (
    <Card className="animate-fade-in gap-4 p-4" aria-label={net?.name ?? container?.name}>
      <div className="flex items-start gap-3">
        <IconTile tone={net ? networkTone(net) : RUNTIME_TONE[rt]}>{net ? <Network /> : <Box />}</IconTile>
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold [overflow-wrap:anywhere]">{net?.name ?? container?.name}</h3>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            {net ? t("network") : t("container")} <HostChip host={host} />
          </div>
        </div>
        <Button variant="ghost" size="icon" className="size-7" onClick={() => onSelect(null)} aria-label={t("clear")}>
          <X />
        </Button>
      </div>
      {net?.detailUnavailable && <p className="text-sm text-warning">{tn("partial.detail")}</p>}
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
        {net ? (
          <>
            <Kv label={t("driver")}>{net.driver}</Kv>
            {net.parent && (
              <Kv label={t("parent")}>
                <span className="font-mono text-xs">{net.parent}</span>
              </Kv>
            )}
            {net.subnet && (
              <Kv label={t("subnet")}>
                <span className="font-mono text-xs">{net.subnet}</span>
              </Kv>
            )}
            {net.gateway && (
              <Kv label={t("gateway")}>
                <span className="font-mono text-xs">{net.gateway}</span>
              </Kv>
            )}
            {net.ipRange && (
              <Kv label={t("ipRange")}>
                <span className="font-mono text-xs">{net.ipRange}</span>
              </Kv>
            )}
            {net.subnet && <Kv label={t("staticIp")}>{net.detailUnavailable ? tn("partial.unknown") : net.subnetConfigured ? t("staticYes") : t("staticNo")}</Kv>}
            <Kv label={t("families")}>{net.detailUnavailable ? tn("partial.unknown") : net.ipv6 ? (net.ipv4 === false ? t("familyV6") : t("familyDual")) : t("familyV4")}</Kv>
            {net.subnet6 && (
              <Kv label={t("subnet6")}>
                <span className="font-mono text-xs break-all">{net.subnet6}</span>
              </Kv>
            )}
            {net.gateway6 && (
              <Kv label={t("gateway6")}>
                <span className="font-mono text-xs break-all">{net.gateway6}</span>
              </Kv>
            )}
            {net.ipRange6 && (
              <Kv label={t("ipRange6")}>
                <span className="font-mono text-xs break-all">{net.ipRange6}</span>
              </Kv>
            )}
            {net.subnet6 && <Kv label={t("staticIp6")}>{net.detailUnavailable ? tn("partial.unknown") : net.subnet6Configured ? t("staticYes") : t("staticNo")}</Kv>}
            {net.detailUnavailable ? <Kv label={t("internal")}>{tn("partial.unknown")}</Kv> : (
            <Kv label={t("internal")}>{net.internal ? t("internalYes") : t("internalNo")}</Kv>
            )}
            <Kv label={t("origin")}>{networkComposeOwner(net) ? t("stack", { name: networkComposeOwner(net)! }) : net.builtIn ? t("standard") : t("own")}</Kv>
          </>
        ) : (
          container && (
            <>
              <Kv label={t("status")}>
                <ToneBadge tone={RUNTIME_TONE[rt]}>{ts(rt)}</ToneBadge>
              </Kv>
              <Kv label={t("image")}>
                <span className="font-mono text-xs">{versionLabel(container.image)}</span>
              </Kv>
              <Kv label={t("cpu")}>{metrics ? formatPercentNumber(metrics.cpu) : "–"}</Kv>
              <Kv label={t("mem")}>{metrics ? formatMem(metrics.memMb) : "–"}</Kv>
              <Kv label={t("ports")}>
                <span className="font-mono text-xs">{formatPorts(container.ports) || "–"}</span>
              </Kv>
            </>
          )
        )}
      </dl>
      {net?.driver === "host" && <p className="text-xs text-muted-foreground">{t("hostMode")}</p>}
      {lan && <p className="text-xs text-muted-foreground">{t("lanNote")}</p>}
      <section>
        <h4 className="mb-1.5 text-xs font-medium text-muted-foreground">{net ? t("attached") : t("memberships")}</h4>
        <ul className="flex flex-col">
          {peers.length === 0 && <li className="text-xs text-muted-foreground">{net?.detailUnavailable ? tn("usage.unknown") : t("noPeers")}</li>}
          {peers.map((p) => {
            const id = net ? p.containerId : p.networkId;
            const pair = { container: containerName(p.containerId), network: name(p.networkId) };
            const modeNet = topology.networks.find((n) => n.id === p.networkId);
            const canDisconnect = disconnect.get(p.networkId)?.available && !modeNet?.detailUnavailable && !modeNet?.inventory?.stale && online && modeNet?.driver !== "host" && modeNet?.driver !== "none";
            return (
              <li key={id} className="flex items-start gap-2 border-t border-border/50 py-1.5 text-sm first:border-0">
                {net ? <Box className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" /> : <Network className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />}
                <button type="button" className="min-w-0 flex-1 text-left [overflow-wrap:anywhere] hover:text-primary-text hover:underline" onClick={() => onSelect(id)}>
                  {name(id)}
                </button>
                {modeNet?.driver === "host" ? (
                  <span className="flex items-center gap-1 font-mono text-xs text-muted-foreground" title={t("hostAddressHint")}>
                    {hostAddr ?? t("hostAddressUnknown")}
                    {hostAddr && <span className="rounded bg-muted px-1 font-sans text-[10px]">{t("hostAddressShort")}</span>}
                  </span>
                ) : (
                  <span className="flex max-w-[60%] min-w-0 flex-col items-end font-mono text-xs text-muted-foreground">
                    {p.ip || p.ip6 ? (
                      <>
                        {p.ip && <span>{p.ip}</span>}
                        {p.ip6 && <span className="text-right [overflow-wrap:anywhere]">{p.ip6}</span>}
                      </>
                    ) : (
                      t("noAddress")
                    )}
                  </span>
                )}
                {(p.staticIp || p.staticIp6) && <span className="rounded bg-muted px-1 text-[10px] text-muted-foreground">{t("static")}</span>}
                {canDisconnect && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="icon" className="size-6" aria-label={t("disconnect", pair)} onClick={() => onConnect({ mode: "disconnect", containerId: p.containerId, networkId: p.networkId })}>
                        <Unlink className="size-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t("disconnect", pair)}</TooltipContent>
                  </Tooltip>
                )}
              </li>
            );
          })}
        </ul>
      </section>
      {container && (
        <section>
          <h4 className="mb-1.5 text-xs font-medium text-muted-foreground">{t("dependsOn")}</h4>
          <DependencyList ids={dependsOn} name={containerName} onSelect={onSelect} empty={t("noDependencies")} />
          <h4 className="mt-3 mb-1.5 text-xs font-medium text-muted-foreground">{t("requiredBy")}</h4>
          <DependencyList ids={requiredBy} name={containerName} onSelect={onSelect} empty={t("noDependencies")} />
          <p className="mt-2 text-[11px] leading-relaxed text-muted-foreground">{t("dependenciesHint")}</p>
        </section>
      )}
      {net && peers.length > 0 && (
        <Button size="sm" variant="secondary" className="self-start" onClick={() => onToggleExpand(net.id)} aria-pressed={expanded.has(net.id)}>
          {expanded.has(net.id) ? <ChevronsDownUp data-icon="inline-start" /> : <ChevronsUpDown data-icon="inline-start" />}
          {expanded.has(net.id) ? t("collapse") : t("expand", { count: peers.length })}
        </Button>
      )}
      <div className="flex flex-wrap gap-2 border-t pt-3">
        {net && net.driver !== "host" && net.driver !== "none" && (
          <Button size="sm" variant="outline" disabled={!connectAvailable || !!net?.detailUnavailable || !!net?.inventory?.stale || !online} title={net?.detailUnavailable ? tn("partial.detail") : [...connect.values()].find((c) => c.reason)?.reason} onClick={() => onConnect({ mode: "connect", networkId: net.id })}>
            <Link2 data-icon="inline-start" />
            {t("connectHere")}
          </Button>
        )}
        {container && (
          <Button size="sm" variant="outline" disabled={!connectAvailable || !!net?.detailUnavailable || !!net?.inventory?.stale || !online} title={net?.detailUnavailable ? tn("partial.detail") : [...connect.values()].find((c) => c.reason)?.reason} onClick={() => onConnect({ mode: "connect", containerId: container.id })}>
            <Link2 data-icon="inline-start" />
            {t("connect")}
          </Button>
        )}
        {(net?.stackId ?? container?.stackId) && (
          <Button size="sm" variant="ghost" asChild>
            <Link to={`/stacks/${net?.stackId ?? container?.stackId}`}>{t("openStack")}</Link>
          </Button>
        )}
        {container?.stackId && (
          <Button size="sm" variant="ghost" asChild>
            <Link to={`/stacks/${container.stackId}?tab=logs&service=${container.serviceName}`}>{t("logs")}</Link>
          </Button>
        )}
        {container && !container.stackId && (
          <Button size="sm" variant="ghost" asChild>
            <Link to={`/containers?c=${container.id}`}>{t("openContainer")}</Link>
          </Button>
        )}
        {net &&
          (blocked ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span tabIndex={0}>{removeButton}</span>
              </TooltipTrigger>
              <TooltipContent className="max-w-64">{tn(`removeBlocked.${blocked}`, { stack: networkComposeOwner(net) ?? "", count: peers.length })}</TooltipContent>
            </Tooltip>
          ) : (
            removeButton
          ))}
      </div>
      {!online && host && <p className="text-xs text-warning">{tn("offlineNote", { name: host.name })}</p>}
    </Card>
  );
}

function DependencyList({ ids, name, onSelect, empty }: { ids: string[]; name: (id: string) => string; onSelect: (id: string) => void; empty: string }) {
  if (!ids.length) return <p className="text-xs text-muted-foreground">{empty}</p>;
  return (
    <ul className="flex flex-col">
      {ids.map((id) => (
        <li key={id} className="flex items-center gap-2 border-t border-border/50 py-1.5 text-sm first:border-0">
          <ArrowRight className="size-3.5 text-muted-foreground" aria-hidden />
          <button type="button" className="min-w-0 flex-1 truncate text-left hover:text-primary-text hover:underline" onClick={() => onSelect(id)}>
            {name(id)}
          </button>
        </li>
      ))}
    </ul>
  );
}
