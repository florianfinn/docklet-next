import { useCapability } from "@/data/use-capability";
import { Boxes, Crosshair, Link2, RotateCcw, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslations } from "use-intl";

import type { ContainerInfo, Host, Topology, TopologyNetwork } from "@/domain/model";
import { NETWORK_GROUPS, networkGroup, removeBlock, type NetworkGroup } from "@/domain/networks";
import { ActionsContextMenu, ActionsMenu, type MenuGroups } from "@/shared/components/action-menu";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { SortableHead } from "@/shared/components/sortable-head";
import { nextSort, sortRows, type SortState } from "@/shared/lib/table-sort";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { BareTable, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";

import { NetworkUsage, type NetworkMember } from "./NetworkUsage";

import type { ConnectRequest } from "./ConnectNetworkDialog";

type GroupFilter = "all" | NetworkGroup;
type SortKey = "order" | "name" | "subnet" | "members";

interface Props {
  host: Host;
  /** Networks of the host in display order. */
  networks: TopologyNetwork[];
  topology: Topology;
  /** Containers of the host; the search also finds the networks of a matching container, as the graph does. */
  containers: ContainerInfo[];
  focus: string | null;
  /** The page's search; the table filters by it too instead of offering a second search field. */
  query: string;
  onClearQuery: () => void;
  onFocus: (id: string) => void;
  onConnect: (request: ConnectRequest) => void;
  onRemove: (network: TopologyNetwork) => void;
}

/**
 * Line breaks only after "_", "." or "-", so a narrow column never splits a
 * name inside a word. <wbr> keeps the text itself unchanged for copy and find.
 */
const breakable = (name: string) => name.split(/(?<=[_.-])/).flatMap((part, i) => (i ? [<wbr key={i} />, part] : [part]));

/** Text the shared search matches: name, driver, parent interface, both subnets and the names of member containers. */
function networkMatches(n: TopologyNetwork, query: string, memberNames: readonly string[] = []): boolean {
  const q = query.trim().toLowerCase();
  return !q || [n.name, n.driver, n.parent, n.subnet, n.subnet6, ...memberNames].some((v) => v?.toLowerCase().includes(q));
}

/**
 * Compact network list of the shown host: name with driver below, IPv4 and
 * IPv6 subnets wrapped in one column, member count and actions, so it fits
 * beside the graph. Filters by the page search and a network group, sorts by
 * its headers, and marks the focused network (or the networks of a focused
 * container) in step with the graph. The ellipsis button and a right click
 * open the same actions.
 */
export function NetworkTable({ host, networks, topology, containers, focus, query, onClearQuery, onFocus, onConnect, onRemove }: Props) {
  const t = useTranslations("networks");
  const [group, setGroup] = useState<GroupFilter>("all");
  const [sort, setSort] = useState<SortState<SortKey>>({ key: "order", dir: "asc" });
  const connect = useCapability("connectNetwork");
  const remove = useCapability("removeNetwork");
  const online = host.status === "online";
  const members = useMemo(() => {
    const byId = new Map(containers.map((c) => [c.id, c]));
    const out = new Map<string, NetworkMember[]>();
    for (const a of topology.attachments) {
      const member = byId.get(a.containerId) ?? { id: a.containerId, name: a.containerId, state: "unknown" as const };
      const group = out.get(a.networkId) ?? [];
      if (!group.some((c) => c.id === member.id)) group.push(member);
      out.set(a.networkId, group);
    }
    return out;
  }, [containers, topology.attachments]);
  const memberNames = useMemo(() => {
    const names = new Map(containers.map((c) => [c.id, [c.name, c.serviceName ?? "", c.stackName ?? c.stackId ?? ""]]));
    const out = new Map<string, string[]>();
    for (const a of topology.attachments) out.set(a.networkId, [...(out.get(a.networkId) ?? []), ...(names.get(a.containerId) ?? [])]);
    return out;
  }, [containers, topology.attachments]);
  const related = useMemo(() => new Set(topology.attachments.filter((a) => a.containerId === focus).map((a) => a.networkId)), [topology.attachments, focus]);

  const shown = useMemo(() => {
    const filtered = networks.filter((n) => (group === "all" || networkGroup(n) === group) && networkMatches(n, query, memberNames.get(n.id)));
    const order = new Map(networks.map((n, i) => [n.id, i]));
    return sortRows(
      filtered,
      sort,
      (n, key) => (key === "order" ? (order.get(n.id) ?? 0) : key === "name" ? n.name : key === "subnet" ? (n.subnet ?? n.subnet6 ?? null) : (members.get(n.id)?.length ?? 0)),
      (n) => ({ id: n.id, name: n.name, hostName: host.name }),
    );
  }, [networks, group, query, sort, members, memberNames, host.name]);
  const filtering = group !== "all" || !!query.trim();
  const reset = () => {
    setGroup("all");
    setSort({ key: "order", dir: "asc" });
    onClearQuery();
  };
  const onSort = (key: SortKey) => setSort((s) => nextSort(s, key, (k) => (k === "members" ? "desc" : "asc")));

  const menu = (n: TopologyNetwork): MenuGroups => [
    [{ id: "focus", label: t("menu.focus"), icon: <Crosshair />, onSelect: () => onFocus(n.id) }],
    [{ id: "connect", label: t("menu.connect"), icon: <Link2 />, disabled: !connect.available || !!n.inventory?.stale || !online || n.driver === "host" || n.driver === "none", onSelect: () => onConnect({ mode: "connect", networkId: n.id }) }],
    [{ id: "remove", label: t("menu.remove"), icon: <Trash2 />, disabled: !remove.available || !!n.inventory?.stale || !online || !!removeBlock(topology, n), title: remove.reason, onSelect: () => onRemove(n) }],
  ];

  return (
    <section aria-labelledby="net-table-title" className="flex min-w-0 flex-col gap-2" data-slot="network-table">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h2 id="net-table-title" className="text-sm font-medium text-muted-foreground">
          {t("table.title", { host: host.name })}
        </h2>
        <span className="text-xs text-muted-foreground tabular-nums" aria-live="polite">
          {t("table.count", { shown: shown.length, total: networks.length })}
        </span>
        {filtering && (
          <Button variant="ghost" size="sm" className="ml-auto h-7 px-2 text-xs" onClick={reset}>
            <RotateCcw data-icon="inline-start" /> {t("table.reset")}
          </Button>
        )}
      </div>
      <SegmentedToggle<GroupFilter>
        value={group}
        onChange={setGroup}
        label={t("table.groupFilter")}
        size="xs"
        fit="content"
        wrap
        options={(["all", ...NETWORK_GROUPS] as const).map((g) => ({ value: g, label: t(`table.groups.${g}`) }))}
      />
      <Card className="gap-0 overflow-hidden py-0">
        <BareTable className="[&_td]:align-top [&_td]:whitespace-normal">
          <TableHeader>
            <TableRow>
              <SortableHead sortKey="name" sort={sort} onSort={onSort} label={t("table.sortBy", { column: t("table.name") })} className="pl-2">
                {t("table.name")}
              </SortableHead>
              <SortableHead sortKey="subnet" sort={sort} onSort={onSort} label={t("table.sortBy", { column: t("table.subnet") })}>
                {t("table.subnet")}
              </SortableHead>
              <SortableHead sortKey="members" sort={sort} onSort={onSort} label={t("table.sortBy", { column: t("table.containers") })} className="w-px text-right">
                <Boxes className="size-3.5" aria-hidden />
                <span className="sr-only">{t("table.containers")}</span>
              </SortableHead>
              <TableHead className="w-px">
                <span className="sr-only">{t("table.actions")}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {shown.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="py-6 text-center text-sm text-muted-foreground">
                  <p>{t("table.empty")}</p>
                  <Button variant="outline" size="sm" className="mt-2" onClick={reset}>
                    <RotateCcw data-icon="inline-start" /> {t("table.reset")}
                  </Button>
                </TableCell>
              </TableRow>
            )}
            {shown.map((n) => {
              const groups = menu(n);
              const selected = focus === n.id;
              return (
                <ActionsContextMenu key={n.id} groups={groups} title={n.name}>
                  <TableRow data-state={selected ? "selected" : undefined} data-related={related.has(n.id) || undefined} className={cn(related.has(n.id) && "bg-control/8")}>
                    <TableCell className="min-w-0 pl-3">
                      <button type="button" className="block max-w-full text-left font-medium break-words hover:text-primary-text hover:underline" onClick={() => onFocus(n.id)} aria-label={t("table.select", { name: n.name })} aria-pressed={selected}>
                        {breakable(n.name)}
                      </button>
                      <span className="mt-0.5 flex flex-wrap items-center gap-1 text-[11px] text-muted-foreground">
                        {n.driver}
                        {n.parent ? ` · ${n.parent}` : ""}
                        {n.internal && <span>· {t("internal")}</span>}
                        {n.builtIn && <span className="rounded bg-muted px-1 text-[10px]">{t("standardShort")}</span>}
                      </span>
                    </TableCell>
                    <TableCell className="font-mono text-[11.5px] leading-snug">
                      {n.subnet || n.subnet6 ? (
                        <span className="flex flex-col">
                          {n.subnet && <span>{n.subnet}</span>}
                          {n.subnet6 && <span className="[overflow-wrap:anywhere]">{n.subnet6}</span>}
                        </span>
                      ) : (
                        <span className="text-muted-foreground">–</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right tabular"><NetworkUsage network={n} members={members.get(n.id) ?? []} /></TableCell>
                    <TableCell className="pr-2 text-right">
                      <ActionsMenu groups={groups} label={t("table.more", { name: n.name })} />
                    </TableCell>
                  </TableRow>
                </ActionsContextMenu>
              );
            })}
          </TableBody>
        </BareTable>
      </Card>
    </section>
  );
}
