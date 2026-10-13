import { MarkerType, type Edge, type Node } from "@xyflow/react";

import type { ContainerInfo, Host, Topology, TopologyNetwork } from "@/domain/model";
import { networkGroup, NETWORK_GROUPS, sortNetworks, type NetworkGroup } from "@/domain/networks";
import { containerRuntime, type RuntimeStatus } from "@/domain/status";

import { hostAddress } from "./host-address";
import { BLOCK_GAP, BUS, containerSize, GUTTER, networkSize, NODE_MIN_W, RAIL_X, type AddressRow, type Bounds } from "./topology-geometry";

export interface NetworkData extends Record<string, unknown> {
  network: TopologyNetwork;
  members: number;
  problems: number;
  expanded: boolean;
  dim: boolean;
  /** Column width; every block of a column shares it. */
  width: number;
  /** y of the free band between the network blocks and the members of its row; membership edges run there. */
  band: number;
}
/** Address of a container in one network, shown on the container block while it or that network has focus. */
export interface FocusAddress {
  networkId: string;
  network: string;
  /** "ip": own address; "host": host network mode, the host's address (null when unknown, e.g. edge); "none": no address. */
  kind: "ip" | "host" | "none";
  /** IPv4 address, or the IPv6 address of an IPv6-only member. */
  address: string | null;
  /** IPv6 address of a dual-stack member, on its own line. */
  address6?: string;
}
export interface ContainerData extends Record<string, unknown> {
  container: ContainerInfo;
  host: Host | undefined;
  status: RuntimeStatus;
  dim: boolean;
  /** Empty without focus; never drawn on edges. */
  addresses: FocusAddress[];
  width: number;
}
export interface LaneData extends Record<string, unknown> {
  /** Network group, or containers of this host without any network. */
  group: NetworkGroup | "isolated";
  count: number;
  width: number;
}

export type TopoNode = Node<NetworkData, "network"> | Node<ContainerData, "container"> | Node<LaneData, "lane">;

const LANE_H = 34;
const LANE_GAP = 28;
/** Room for the lane heading (group and count). */
const LANE_MIN_W = 280;
const ROW_GAP = 40;
const NET_GAP = 14;
/** Text of an unknown host address on the block; its length only reserves width. */
const UNKNOWN_ADDRESS = "Host-Adresse unbekannt";
const NO_ADDRESSES: FocusAddress[] = [];

const PROBLEM: ReadonlySet<RuntimeStatus> = new Set(["unhealthy", "restarting", "unknown"]);

export interface HostLayoutInput {
  topology: Topology;
  containers: ContainerInfo[];
  host: Host;
  /** Networks whose containers are shown; others stay one aggregated node. */
  expanded: ReadonlySet<string>;
  /** Widest lane row in layout px that stays legible on the canvas (see layoutBudget); unknown: Infinity. */
  budget?: number;
}

export interface Membership {
  containerId: string;
  networkId: string;
  ip?: string;
  ip6?: string;
  internal: boolean;
  network: string;
  driver: TopologyNetwork["driver"];
}

export interface HostLayout {
  nodes: TopoNode[];
  /** Container → network memberships of placed containers (edges and focus); never drawn as a chain. */
  memberships: Membership[];
  /** Host address for containers in host network mode; null for edge hosts. */
  hostAddress: string | null;
  /** Configured dependencies between placed containers. */
  dependencies: Topology["dependencies"];
  /** Container id → network it is drawn under; used to expand a collapsed network on selection. */
  homeOf: ReadonlyMap<string, string>;
  /** Extent of all blocks, for the canvas height and the fit. */
  bounds: Bounds;
  stats: { networks: number; containers: number; shown: number };
}


/**
 * Network-first layout of ONE host. Networks sit in lanes – LAN networks
 * (macvlan/ipvlan), then user and stack bridges, then Docker's own – each
 * sorted by name, optionally with its containers stacked below. A container
 * is drawn under its first network in that order and linked to the others;
 * positions never encode traffic. Pure and memoised by the caller; search,
 * focus and filters only decorate the result (see decorate).
 */
export function layoutHost({ topology, containers, host, expanded, budget = Infinity }: HostLayoutInput): HostLayout {
  if (typeof performance !== "undefined") performance.mark("topology-layout:start");
  const byId = new Map(containers.filter((c) => c.hostId === host.id).map((c) => [c.id, c]));
  const nets = sortNetworks(topology.networks.filter((n) => n.hostId === host.id));
  const netIds = new Set(nets.map((n) => n.id));
  const attachments = topology.attachments.filter((a) => byId.has(a.containerId) && netIds.has(a.networkId));
  const membersOf = new Map<string, string[]>();
  for (const a of attachments) membersOf.set(a.networkId, [...(membersOf.get(a.networkId) ?? []), a.containerId]);
  const homeOf = new Map<string, string>();
  const links = new Map<string, number>();
  for (const n of nets) {
    for (const id of membersOf.get(n.id) ?? []) {
      // Home: the container's own stack network, otherwise its first network in display order.
      const own = !!n.stackId && byId.get(id)?.stackId === n.stackId;
      const current = nets.find((x) => x.id === homeOf.get(id));
      if (!current || (own && current.stackId !== n.stackId)) homeOf.set(id, n.id);
      links.set(id, (links.get(id) ?? 0) + 1);
    }
  }
  // Every block is sized for its widest focus state (all memberships, both families), so focus never moves anything.
  const hostAddr = hostAddress(host);
  const netById = new Map(nets.map((n) => [n.id, n]));
  const rowsOf = new Map<string, AddressRow[]>();
  for (const a of attachments) {
    const n = netById.get(a.networkId)!;
    const values = n.driver === "host" ? [hostAddr ?? UNKNOWN_ADDRESS] : [a.ip, a.ip6].filter((v): v is string => !!v);
    rowsOf.set(a.containerId, [...(rowsOf.get(a.containerId) ?? []), { network: n.name, values, host: n.driver === "host" }]);
  }
  const sizeOf = new Map(
    [...byId.values()].map((c) => {
      const status = containerRuntime(c, host);
      return [c.id, { status, ...containerSize({ title: c.serviceName ?? c.name, meta: `${c.stackId ?? c.origin} · ${status.padEnd(12)}`, rows: rowsOf.get(c.id) ?? [] }) }];
    }),
  );
  const netSize = new Map(nets.map((n) => [n.id, networkSize(n)]));
  // Containers in several networks sit first under their column, so links to other networks stay short.
  const order = (a: ContainerInfo, b: ContainerInfo) => (links.get(b.id) ?? 0) - (links.get(a.id) ?? 0) || (a.stackId ?? "").localeCompare(b.stackId ?? "") || (a.serviceName ?? a.name).localeCompare(b.serviceName ?? b.name);
  const container = (c: ContainerInfo, x: number, y: number, width: number): TopoNode => ({ id: c.id, type: "container", position: { x, y }, data: { container: c, host, status: sizeOf.get(c.id)!.status, dim: false, addresses: NO_ADDRESSES, width } });

  // Columns per row: as many as the canvas' width budget holds, else (width unknown) a near-square grid of at most six.
  const perRow = Number.isFinite(budget) ? Infinity : Math.max(1, Math.min(6, Math.ceil(Math.sqrt(nets.length * 1.6))));
  const pack = <T extends { width: number }>(items: T[]) => {
    const rows: { items: T[]; xs: number[]; width: number }[] = [];
    for (const item of items) {
      const row = rows.at(-1);
      if (row && row.items.length < perRow && row.width + GUTTER + item.width <= budget) {
        row.xs.push(row.width + GUTTER);
        row.items.push(item);
        row.width += GUTTER + item.width;
      } else rows.push({ items: [item], xs: [0], width: item.width });
    }
    return rows;
  };
  const nodes: TopoNode[] = [];
  const placed = new Set<string>();
  let y = 0;
  let right = 0;
  const lane = (group: LaneData["group"], count: number, rows: { width: number }[]) => {
    const width = Math.max(LANE_MIN_W, ...rows.map((r) => r.width));
    right = Math.max(right, width);
    nodes.push({ id: `lane-${group}`, type: "lane", position: { x: 0, y }, selectable: false, focusable: false, draggable: false, data: { group, count, width } });
    y += LANE_H;
  };
  for (const group of NETWORK_GROUPS) {
    const own = nets.filter((n) => networkGroup(n) === group);
    if (!own.length) continue;
    const columns = own.map((n) => {
      const members = expanded.has(n.id) ? (membersOf.get(n.id) ?? []).filter((id) => homeOf.get(id) === n.id).map((id) => byId.get(id)!).toSorted(order) : [];
      members.forEach((m) => placed.add(m.id));
      return { net: n, members, width: Math.max(NODE_MIN_W, netSize.get(n.id)!.width, ...members.map((m) => sizeOf.get(m.id)!.width)) };
    });
    const rows = pack(columns);
    lane(group, own.length, rows);
    for (const row of rows) {
      // Members of a row start below the tallest network block of the row.
      const head = Math.max(...row.items.map((c) => netSize.get(c.net.id)!.height)) + NET_GAP;
      let bottom = y;
      row.items.forEach((col, i) => {
        const x = row.xs[i]!;
        const all = membersOf.get(col.net.id) ?? [];
        const problems = all.filter((id) => PROBLEM.has(containerRuntime(byId.get(id)!, host))).length;
        nodes.push({ id: col.net.id, type: "network", position: { x, y }, data: { network: col.net, members: all.length, problems, expanded: expanded.has(col.net.id), dim: false, width: col.width, band: y + head - NET_GAP / 2 } });
        let cy = y + head;
        for (const c of col.members) {
          nodes.push(container(c, x, cy, col.width));
          cy += sizeOf.get(c.id)!.reserved;
        }
        bottom = Math.max(bottom, col.members.length ? cy - BLOCK_GAP : y + netSize.get(col.net.id)!.height);
      });
      y = bottom + ROW_GAP;
    }
    y += LANE_GAP - ROW_GAP;
  }

  // Containers without any network (network_mode none, all disconnected) get their own lane instead of vanishing.
  const isolated = [...byId.values()].filter((c) => !links.has(c.id)).toSorted((a, b) => (a.serviceName ?? a.name).localeCompare(b.serviceName ?? b.name));
  if (isolated.length) {
    const rows = pack(isolated.map((c) => ({ c, width: sizeOf.get(c.id)!.width })));
    lane("isolated", isolated.length, rows);
    for (const row of rows) {
      row.items.forEach(({ c, width }, i) => {
        placed.add(c.id);
        nodes.push(container(c, row.xs[i]!, y, width));
      });
      y += Math.max(...row.items.map(({ c }) => sizeOf.get(c.id)!.reserved)) - BLOCK_GAP + ROW_GAP;
    }
    y -= ROW_GAP;
  } else {
    y -= LANE_GAP;
  }

  const memberships = attachments
    .filter((a) => placed.has(a.containerId))
    .map((a) => {
      const n = netById.get(a.networkId)!;
      return { containerId: a.containerId, networkId: a.networkId, ip: a.ip, ip6: a.ip6, internal: n.internal, network: n.name, driver: n.driver };
    });
  const dependencies = topology.dependencies.filter((d) => placed.has(d.from) && placed.has(d.to));
  // The bounds include the buses left of the first column and, for memberships between lane rows, the rail.
  const bandOf = new Map(nodes.flatMap((n) => (n.type === "network" ? [[n.id, n.data.band] as const] : [])));
  const left = memberships.some((m) => bandOf.get(m.networkId) !== bandOf.get(homeOf.get(m.containerId) ?? "")) ? RAIL_X - 4 : -BUS - 4;
  if (typeof performance !== "undefined") {
    performance.measure("topology-layout", "topology-layout:start");
    performance.clearMarks("topology-layout:start");
  }
  return { nodes, memberships, hostAddress: hostAddr, dependencies, homeOf, bounds: { x: left, y: 0, width: right - left, height: Math.max(0, y) }, stats: { networks: nets.length, containers: byId.size, shown: placed.size } };
}

export interface DecorateInput {
  query: string;
  onlyProblems: boolean;
  focus: string | null;
  showDependencies: boolean;
}

export function focusAddress(m: Membership, host: string | null): FocusAddress {
  if (m.driver === "host") return { networkId: m.networkId, network: m.network, kind: "host", address: host };
  if (m.driver === "none" || (!m.ip && !m.ip6)) return { networkId: m.networkId, network: m.network, kind: "none", address: null };
  // Dual stack shows both known addresses on two lines; an unknown one is left out, never derived.
  const ip: FocusAddress = { networkId: m.networkId, network: m.network, kind: "ip", address: m.ip ?? m.ip6 ?? null };
  return m.ip && m.ip6 ? { ...ip, address6: m.ip6 } : ip;
}

/**
 * Addresses shown on container blocks for a focus: a focused container lists
 * all its networks, a focused network shows each member's address in it.
 */
export function focusAddresses(layout: HostLayout, focus: string | null): Map<string, FocusAddress[]> {
  const out = new Map<string, FocusAddress[]>();
  if (!focus) return out;
  for (const m of layout.memberships) {
    if (m.containerId !== focus && m.networkId !== focus) continue;
    out.set(m.containerId, [...(out.get(m.containerId) ?? []), focusAddress(m, layout.hostAddress)]);
  }
  return out;
}

const sameAddresses = (a: FocusAddress[], b: FocusAddress[]) => a.length === b.length && a.every((x, i) => x.networkId === b[i]?.networkId && x.address === b[i]?.address && x.address6 === b[i]?.address6 && x.kind === b[i]?.kind);

/** Cheap pass over a finished layout: dimming, selection, focus addresses and edges. */
export function decorate(layout: HostLayout, { query, onlyProblems, focus, showDependencies }: DecorateInput): { nodes: TopoNode[]; edges: Edge[] } {
  const q = query.trim().toLowerCase();
  const neighbours = new Set<string>();
  if (focus) {
    neighbours.add(focus);
    for (const m of layout.memberships) {
      if (m.containerId === focus) neighbours.add(m.networkId);
      if (m.networkId === focus) neighbours.add(m.containerId);
    }
    for (const d of layout.dependencies) {
      if (d.from === focus) neighbours.add(d.to);
      if (d.to === focus) neighbours.add(d.from);
    }
  }
  const addressesOf = focusAddresses(layout, focus);
  const nodes = layout.nodes.map((n): TopoNode => {
    if (n.type === "lane") return n;
    const selected = n.id === focus;
    if (n.type === "network") {
      const d = n.data;
      const hay = `${d.network.name} ${d.network.driver} ${d.network.parent ?? ""} ${d.network.subnet ?? ""}`.toLowerCase();
      const dim = (!!focus && !neighbours.has(n.id)) || (!!q && !hay.includes(q)) || (onlyProblems && d.problems === 0);
      return dim === d.dim && !!n.selected === selected ? n : { ...n, selected, data: { ...d, dim } };
    }
    const d = n.data;
    const hay = `${d.container.name} ${d.container.stackId ?? ""}`.toLowerCase();
    const dim = (!!focus && !neighbours.has(n.id)) || (!!q && !hay.includes(q)) || (onlyProblems && !PROBLEM.has(d.status));
    const found = addressesOf.get(n.id);
    // Unchanged nodes keep their identity, so memoised node components skip the repaint.
    const addresses = !found ? NO_ADDRESSES : sameAddresses(found, d.addresses) ? d.addresses : found;
    return dim === d.dim && !!n.selected === selected && addresses === d.addresses ? n : { ...n, selected, data: { ...d, dim, addresses } };
  });
  const xOf = new Map(layout.nodes.map((n) => [n.id, n.position.x]));
  const edges: Edge[] = [];
  // Membership: a bus on the left of each column that branches into every member from the side. Members
  // stacked below a network never connect top to bottom, so the column cannot read as a chain; links to
  // other columns run through free bands (see membershipPoints), never behind other blocks.
  const bandOf = new Map(layout.nodes.flatMap((n) => (n.type === "network" ? [[n.id, n.data.band] as const] : [])));
  for (const m of layout.memberships) {
    const active = !!focus && (m.containerId === focus || m.networkId === focus);
    const from = bandOf.get(m.networkId) ?? 0;
    edges.push({
      id: `m-${m.networkId}-${m.containerId}`,
      source: m.networkId,
      target: m.containerId,
      sourceHandle: "m",
      targetHandle: "m",
      type: "membership",
      className: "edge-membership",
      data: { from, to: bandOf.get(layout.homeOf.get(m.containerId) ?? "") ?? from },
      style: { stroke: m.internal ? "var(--violet)" : "var(--cyan)", strokeWidth: active ? 2.4 : 1.3, opacity: focus && !active ? 0.2 : 0.7 },
    });
  }
  // Dependency: one dashed arrow per configured depends_on, from the waiting container to the one it waits for.
  if (showDependencies) {
    for (const dep of layout.dependencies) {
      const active = !!focus && (dep.from === focus || dep.to === focus);
      const fromX = xOf.get(dep.from) ?? 0;
      const toX = xOf.get(dep.to) ?? 0;
      edges.push({
        id: `d-${dep.from}-${dep.to}`,
        source: dep.from,
        target: dep.to,
        sourceHandle: "r",
        // Same column or to the left: arrive on the right side; to the right: on the upper left, above the bus branch.
        targetHandle: toX > fromX ? "l" : "rt",
        type: "default",
        className: "edge-dependency",
        markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14, color: "var(--foreground)" },
        style: { stroke: "var(--foreground)", strokeWidth: active ? 2 : 1.3, strokeDasharray: "5 4", opacity: focus && !active ? 0.15 : 0.8 },
      });
    }
  }
  return { nodes, edges };
}
