// Docker network rules shared by the network wizard and every adapter: IPv4
// subnets, IPv6 prefixes (ipv6.ts), creatable drivers, connect/disconnect
// checks and the display order.

import { checkCidr6, checkSubnet6, cidr6Overlap, cidr6Within, formatIp6, inCidr6, nextFreeIp6, parseCidr6, poolFullyScanned6, parseIp6, usableIn6, type Cidr6, type Cidr6Error, type Subnet6Error } from "./ipv6";
import type { ContainerOrigin, NetworkAuxAddress, NetworkAttachment, NetworkDriver, Topology, TopologyNetwork } from "./model";

/** Drivers the hub creates; host and none exist once per Docker host, overlay needs Swarm. */
export const CREATABLE_DRIVERS = ["bridge", "macvlan", "ipvlan"] as const;
export type CreatableDriver = (typeof CREATABLE_DRIVERS)[number];

/** Names of Docker's own networks; user networks may not take them. */
export const RESERVED_NETWORK_NAMES: ReadonlySet<string> = new Set(["bridge", "host", "none", "default", "ingress"]);

const NAME = /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$/;
const INTERFACE = /^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,14}$/;

// --- IPv4 ------------------------------------------------------------------

export function parseIp(text: string): number | undefined {
  const parts = text.trim().split(".");
  if (parts.length !== 4) return undefined;
  let n = 0;
  for (const p of parts) {
    if (!/^\d{1,3}$/.test(p)) return undefined;
    const v = Number(p);
    if (v > 255) return undefined;
    n = n * 256 + v;
  }
  return n;
}

export function formatIp(n: number): string {
  return [24, 16, 8, 0].map((s) => Math.floor(n / 2 ** s) % 256).join(".");
}

export interface Cidr {
  network: number;
  prefix: number;
  /** Number of addresses in the block. */
  size: number;
}

/** "a.b.c.d/p" with the address on the block boundary; host bits set are refused like Docker does. */
export function parseCidr(text: string): Cidr | undefined {
  const [addr = "", bits, ...rest] = text.trim().split("/");
  if (rest.length || bits === undefined || !/^\d{1,2}$/.test(bits)) return undefined;
  const prefix = Number(bits);
  const ip = parseIp(addr);
  if (ip === undefined || prefix < 8 || prefix > 30) return undefined;
  const size = 2 ** (32 - prefix);
  if (ip % size !== 0) return undefined;
  return { network: ip, prefix, size };
}

export function inCidr(ip: number, c: Cidr): boolean {
  return ip >= c.network && ip < c.network + c.size;
}

export function cidrOverlap(a: Cidr, b: Cidr): boolean {
  return a.network < b.network + b.size && b.network < a.network + a.size;
}

/** Usable host address: inside the block, neither network nor broadcast address. */
export function usableIn(ip: number, c: Cidr): boolean {
  return ip > c.network && ip < c.network + c.size - 1;
}

/** First free address of the IP range (or subnet), skipping gateway and taken addresses. */
export function nextFreeIp(network: Pick<TopologyNetwork, "subnet" | "gateway" | "ipRange" | "auxAddresses">, taken: readonly string[]): string | undefined {
  const subnet = network.subnet ? parseCidr(network.subnet) : undefined;
  if (!subnet) return undefined;
  const range = (network.ipRange && parseCidr(network.ipRange)) || subnet;
  const used = new Set([...taken, ...(network.auxAddresses ?? []).map((a) => a.ip), network.gateway ?? ""].map((x) => parseIp(x)).filter((x): x is number => x !== undefined));
  for (let ip = range.network; ip < range.network + range.size; ip++) if (usableIn(ip, subnet) && !used.has(ip)) return formatIp(ip);
  return undefined;
}

// --- Create ----------------------------------------------------------------

export interface NetworkDraft {
  /** Omitted collections are empty for existing callers. Parent remains a separate field. */
  labels?: Record<string, string>;
  options?: Record<string, string>;
  /** One list per family; reservation names are unique within that family. */
  auxAddresses?: NetworkAuxAddress[];
  auxAddresses6?: NetworkAuxAddress[];
  hostId: string;
  name: string;
  driver: CreatableDriver;
  /** IPv4 on unless false. Off with `ipv6` on gives an IPv6-only network; the IPv4 fields are ignored then. */
  ipv4?: boolean;
  /** Docker --ipv6: dual stack with `ipv4`, IPv6-only without. */
  ipv6?: boolean;
  /** Empty: Docker picks a free block (bridge only). */
  subnet: string;
  gateway: string;
  ipRange: string;
  /** Empty with IPv6 on: Docker assigns a prefix from its pool (bridge only). */
  subnet6?: string;
  gateway6?: string;
  ipRange6?: string;
  parent: string;
  internal: boolean;
}

export type NetworkFieldError =
  | "duplicate"
  | "gateway"
  | "required"
  | "invalid"
  | "reserved"
  | "exists"
  | "overlap"
  | "outside"
  | "network-address"
  | "needs-subnet"
  | "parent-in-use"
  | "no-family"
  | Exclude<Cidr6Error, "invalid">
  | Subnet6Error;

export type NetworkDraftErrors = Partial<Record<"labels" | "options" | "auxAddresses" | "auxAddresses6" | "name" | "family" | "subnet" | "gateway" | "ipRange" | "subnet6" | "gateway6" | "ipRange6" | "parent", NetworkFieldError>>;

/** Address families of a draft; older callers without the fields create IPv4-only networks. */
export const draftFamilies = (d: Pick<NetworkDraft, "ipv4" | "ipv6">) => ({ ipv4: d.ipv4 !== false, ipv6: d.ipv6 === true });

/** IPv4 is on unless the network says otherwise. */
export const hasIpv4 = (n: Pick<TopologyNetwork, "ipv4">) => n.ipv4 !== false;

/**
 * Field errors of a network draft against the networks already on its host.
 * macvlan and ipvlan join a LAN, so they need parent, subnet and gateway of
 * every enabled address family. IPv4 and IPv6 are checked independently.
 */
export function validateNetworkDraft(d: NetworkDraft, networks: readonly TopologyNetwork[]): NetworkDraftErrors {
  const onHost = networks.filter((n) => n.hostId === d.hostId);
  const e: NetworkDraftErrors = {};
  const name = d.name.trim();
  if (!name) e.name = "required";
  else if (!NAME.test(name)) e.name = "invalid";
  else if (RESERVED_NETWORK_NAMES.has(name.toLowerCase())) e.name = "reserved";
  else if (onHost.some((n) => n.name === name)) e.name = "exists";

  e.labels = validateNetworkEntries(Object.entries(d.labels ?? {}).map(([key, value]) => ({ key, value })));
  e.options = validateNetworkEntries(Object.entries(d.options ?? {}).map(([key, value]) => ({ key, value })));
  if (draftFamilies(d).ipv4) e.auxAddresses = validateAuxAddresses(d.auxAddresses ?? [], d.subnet, d.gateway, false);
  if (draftFamilies(d).ipv6) e.auxAddresses6 = validateAuxAddresses(d.auxAddresses6 ?? [], d.subnet6 ?? "", d.gateway6 ?? "", true);

  const lan = d.driver !== "bridge";
  if (!draftFamilies(d).ipv4 && !draftFamilies(d).ipv6) e.family = "no-family";
  if (draftFamilies(d).ipv4) validateIpv4(d, onHost, e);
  if (draftFamilies(d).ipv6) validateIpv6(d, onHost, e);

  if (lan) {
    const parent = d.parent.trim();
    if (!parent) e.parent = "required";
    else if (!INTERFACE.test(parent)) e.parent = "invalid";
    // One macvlan network per parent interface; a second one would need an 802.1q sub-interface.
    else if (d.driver === "macvlan" && onHost.some((n) => n.driver === "macvlan" && n.parent === parent)) e.parent = "parent-in-use";
  }
  return Object.fromEntries(Object.entries(e).filter(([, error]) => error));
}

/** Validate editor rows before converting to a record, which would discard duplicate keys. */
export function validateNetworkEntries(entries: readonly { key: string; value: string }[]): NetworkFieldError | undefined {
  const keys = entries.map((entry) => entry.key.trim());
  if (keys.some((key) => !key)) return "required";
  if (new Set(keys).size !== keys.length) return "duplicate";
}

function validateAuxAddresses(entries: readonly NetworkAuxAddress[], subnetText: string, gatewayText: string, ipv6: boolean): NetworkFieldError | undefined {
  if (!entries.length) return;
  const names = entries.map((a) => a.name.trim());
  if (names.some((name) => !name)) return "required";
  if (new Set(names).size !== names.length) return "duplicate";
  const subnet = ipv6 ? parseCidr6(subnetText) : parseCidr(subnetText);
  if (!subnet) return "needs-subnet";
  const addresses = new Set<number | bigint>();
  for (const entry of entries) {
    const ip = ipv6 ? parseIp6(entry.ip) : parseIp(entry.ip);
    if (ip === undefined) return "invalid";
    const inside = ipv6 ? inCidr6(ip as bigint, subnet as Cidr6) : inCidr(ip as number, subnet as Cidr);
    if (!inside) return "outside";
    const reserved = ip === subnet.network || (!ipv6 && ip === (subnet as Cidr).network + (subnet as Cidr).size - 1);
    if (reserved) return "network-address";
    const gateway = ipv6 ? parseIp6(gatewayText) : parseIp(gatewayText);
    if (ip === gateway) return "gateway";
    if (addresses.has(ip)) return "duplicate";
    addresses.add(ip);
  }
}

const DNS_LABEL = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/;

/** DNS aliases are single labels; invalid/empty entries are refused before deduplication. */
export function validateNetworkAliases(aliases: readonly string[]): { aliases: string[]; aliasError?: "invalid" } {
  const normalized = aliases.map((a) => a.trim().toLowerCase());
  if (normalized.some((a) => !DNS_LABEL.test(a))) return { aliases: [], aliasError: "invalid" };
  return { aliases: [...new Set(normalized)] };
}

function validateIpv4(d: NetworkDraft, onHost: readonly TopologyNetwork[], e: NetworkDraftErrors): void {
  const lan = d.driver !== "bridge";
  const subnet = d.subnet.trim() ? parseCidr(d.subnet) : undefined;
  if (!d.subnet.trim()) {
    if (lan) e.subnet = "required";
  } else if (!subnet) e.subnet = "invalid";
  else {
    const clash = onHost.find((n) => {
      const other = n.subnet ? parseCidr(n.subnet) : undefined;
      return !!other && cidrOverlap(subnet, other);
    });
    if (clash) e.subnet = "overlap";
  }

  if (!d.gateway.trim()) {
    if (lan) e.gateway = "required";
  } else {
    const gw = parseIp(d.gateway);
    if (gw === undefined) e.gateway = "invalid";
    else if (!subnet) {
      if (!e.subnet) e.gateway = "needs-subnet";
    } else if (!inCidr(gw, subnet)) e.gateway = "outside";
    else if (!usableIn(gw, subnet)) e.gateway = "network-address";
  }

  if (d.ipRange.trim()) {
    const range = parseCidr(d.ipRange);
    if (!range) e.ipRange = "invalid";
    else if (!subnet) {
      if (!e.subnet) e.ipRange = "needs-subnet";
    } else if (range.network < subnet.network || range.network + range.size > subnet.network + subnet.size) e.ipRange = "outside";
  }
}

function validateIpv6(d: NetworkDraft, onHost: readonly TopologyNetwork[], e: NetworkDraftErrors): void {
  const lan = d.driver !== "bridge";
  const subnet6 = d.subnet6?.trim() ?? "";
  const gateway6 = d.gateway6?.trim() ?? "";
  const range6 = d.ipRange6?.trim() ?? "";
  let subnet: Cidr6 | undefined;
  if (!subnet6) {
    if (lan) e.subnet6 = "required";
  } else {
    const r = checkCidr6(subnet6);
    const policy = r.ok ? checkSubnet6(r.cidr) : undefined;
    if (!r.ok) e.subnet6 = r.error;
    else if (policy) e.subnet6 = policy;
    else {
      subnet = r.cidr;
      const clash = onHost.find((n) => {
        const other = n.subnet6 ? parseCidr6(n.subnet6) : undefined;
        return !!other && cidr6Overlap(r.cidr, other);
      });
      if (clash) e.subnet6 = "overlap";
    }
  }

  if (!gateway6) {
    if (lan) e.gateway6 = "required";
  } else {
    const gw = parseIp6(gateway6);
    if (gw === undefined) e.gateway6 = "invalid";
    else if (!subnet) {
      if (!e.subnet6) e.gateway6 = "needs-subnet";
    } else if (!inCidr6(gw, subnet)) e.gateway6 = "outside";
    else if (!usableIn6(gw, subnet)) e.gateway6 = "network-address";
  }

  if (range6) {
    const r = checkCidr6(range6);
    if (!r.ok) e.ipRange6 = r.error;
    else if (!subnet) {
      if (!e.subnet6) e.ipRange6 = "needs-subnet";
    } else if (!cidr6Within(r.cidr, subnet)) e.ipRange6 = "outside";
    else if (r.cidr.size === 1n && !usableIn6(r.cidr.network, subnet)) e.ipRange6 = "network-address";
  }
}

export function hasErrors(e: object): boolean {
  return Object.values(e).some(Boolean);
}

// --- Connect and disconnect ------------------------------------------------

export interface NetworkContainer {
  id: string;
  name: string;
  hostId: string;
  origin: ContainerOrigin;
  stackId?: string;
}

/** Hard reasons Docker or Docklet refuse a connect; the UI offers no confirm button then. */
export type ConnectBlock = "host-mismatch" | "builtin-network" | "network-mode" | "already-connected";
export type IpError = "invalid" | "outside" | "network-address" | "gateway" | "taken" | "static-unsupported" | "family-disabled" | "pool-exhausted";
/** Effects shown in the review step; they do not block. */
export type NetworkWarning = "compose-runtime" | "unraid-template" | "last-network" | "dependents" | "internal";

const isModeNetwork = (n: Pick<TopologyNetwork, "driver">) => n.driver === "host" || n.driver === "none";

export function attachmentsOf(topology: Pick<Topology, "attachments">, containerId: string): NetworkAttachment[] {
  return topology.attachments.filter((a) => a.containerId === containerId);
}

/** Origin-specific note: Compose and Unraid rebuild the container from their own definition. */
function originWarning(c: NetworkContainer): NetworkWarning[] {
  if (c.origin === "compose") return ["compose-runtime"];
  if (c.origin === "unraid") return ["unraid-template"];
  return [];
}

export interface ConnectCheck {
  aliases?: string[];
  aliasError?: "invalid";
  block?: ConnectBlock;
  ipError?: IpError;
  ip6Error?: IpError;
  warnings: NetworkWarning[];
  /** Address the connect will get: the requested static one or the next free pool address. */
  ip?: string;
  /** IPv6 address in networks with IPv6, canonical; same rules as `ip`. */
  ip6?: string;
}

/** Static IPv6 address check against the network's prefix, gateway and taken addresses (any notation). */
function checkIp6(network: TopologyNetwork, taken: readonly string[], wanted: string): { ip6?: string; ip6Error?: IpError } {
  if (!network.ipv6) return wanted ? { ip6Error: "family-disabled" } : {};
  if (!wanted) {
    const ip6 = nextFreeIp6(network, taken);
    return !ip6 && poolFullyScanned6(network) ? { ip6Error: "pool-exhausted" } : { ip6 };
  }
  const subnet = network.subnet6 ? parseCidr6(network.subnet6) : undefined;
  const value = parseIp6(wanted);
  if (!network.subnet6Configured || !subnet) return { ip6Error: "static-unsupported" };
  if (value === undefined) return { ip6Error: "invalid" };
  if (!inCidr6(value, subnet)) return { ip6Error: "outside" };
  if (!usableIn6(value, subnet)) return { ip6Error: "network-address" };
  if (network.gateway6 && parseIp6(network.gateway6) === value) return { ip6Error: "gateway" };
  if (taken.some((t) => parseIp6(t) === value)) return { ip6Error: "taken" };
  return { ip6: formatIp6(value) };
}

export function checkConnect(topology: Pick<Topology, "networks" | "attachments">, container: NetworkContainer, network: TopologyNetwork, requestedIp = "", requestedIp6 = "", aliases: readonly string[] = []): ConnectCheck {
  const own = attachmentsOf(topology, container.id);
  const netOf = (id: string) => topology.networks.find((n) => n.id === id);
  let block: ConnectBlock | undefined;
  if (network.hostId !== container.hostId) block = "host-mismatch";
  else if (isModeNetwork(network)) block = "builtin-network";
  else if (own.some((a) => { const n = netOf(a.networkId); return !!n && isModeNetwork(n); })) block = "network-mode";
  else if (own.some((a) => a.networkId === network.id)) block = "already-connected";

  const taken = [...topology.attachments.filter((a) => a.networkId === network.id && a.ip).map((a) => a.ip as string), ...(network.auxAddresses ?? []).map((a) => a.ip)];
  let ipError: IpError | undefined;
  let ip: string | undefined;
  const wanted = requestedIp.trim();
  if (!hasIpv4(network)) {
    if (wanted) ipError = "family-disabled";
  } else if (wanted) {
    const subnet = network.subnet ? parseCidr(network.subnet) : undefined;
    const value = parseIp(wanted);
    if (!network.subnetConfigured || !subnet) ipError = "static-unsupported";
    else if (value === undefined) ipError = "invalid";
    else if (!inCidr(value, subnet)) ipError = "outside";
    else if (!usableIn(value, subnet)) ipError = "network-address";
    else if (network.gateway && parseIp(network.gateway) === value) ipError = "gateway";
    else if (taken.some((t) => parseIp(t) === value)) ipError = "taken";
    else ip = formatIp(value);
  } else {
    ip = nextFreeIp(network, taken);
    if (!ip && network.subnet && parseCidr(network.subnet)) ipError = "pool-exhausted";
  }

  const taken6 = [...topology.attachments.filter((a) => a.networkId === network.id && a.ip6).map((a) => a.ip6 as string), ...(network.auxAddresses6 ?? []).map((a) => a.ip)];
  const { ip6, ip6Error } = checkIp6(network, taken6, requestedIp6.trim());

  const warnings = [...originWarning(container), ...(network.internal ? (["internal"] as const) : [])];
  return { block, ipError, ip6Error, warnings, ip, ip6, ...(aliases.length ? validateNetworkAliases(aliases) : {}) };
}

export interface DisconnectCheck {
  block?: "not-connected" | "network-mode";
  warnings: NetworkWarning[];
  /** Containers that depend on this one and reach it only through this network. */
  dependents: string[];
}

export function checkDisconnect(topology: Topology, container: NetworkContainer, network: TopologyNetwork): DisconnectCheck {
  const own = attachmentsOf(topology, container.id);
  if (!own.some((a) => a.networkId === network.id)) return { block: "not-connected", warnings: [], dependents: [] };
  if (isModeNetwork(network)) return { block: "network-mode", warnings: [], dependents: [] };
  const remaining = new Set(own.filter((a) => a.networkId !== network.id).map((a) => a.networkId));
  const dependents = topology.dependencies
    .filter((d) => d.to === container.id)
    .map((d) => d.from)
    .filter((from) => {
      const theirs = attachmentsOf(topology, from).map((a) => a.networkId);
      return theirs.includes(network.id) && !theirs.some((id) => remaining.has(id));
    });
  const warnings: NetworkWarning[] = [...originWarning(container)];
  if (remaining.size === 0) warnings.push("last-network");
  if (dependents.length) warnings.push("dependents");
  return { warnings, dependents };
}

/** Removal is refused for Docker's own networks, Compose-owned ones and while any container (also stopped) is attached. */
export function removeBlock(topology: Pick<Topology, "attachments">, network: TopologyNetwork): "builtin-network" | "compose-managed" | "in-use" | undefined {
  if (network.builtIn) return "builtin-network";
  if (network.stackId) return "compose-managed";
  if (topology.attachments.some((a) => a.networkId === network.id)) return "in-use";
  return undefined;
}

// --- Order -----------------------------------------------------------------

/** LAN networks first, then user and stack bridges, Docker's own networks last. */
export type NetworkGroup = "lan" | "custom" | "standard";
export const NETWORK_GROUPS: readonly NetworkGroup[] = ["lan", "custom", "standard"];

export function networkGroup(n: Pick<TopologyNetwork, "driver" | "builtIn">): NetworkGroup {
  if (n.builtIn) return "standard";
  return n.driver === "macvlan" || n.driver === "ipvlan" ? "lan" : "custom";
}

const DRIVER_ORDER: Record<NetworkDriver, number> = { macvlan: 0, ipvlan: 1, bridge: 2, overlay: 3, host: 4, none: 5, null: 5 };

/** Stable display order: group, then stack-less before stack networks, then name. */
export function sortNetworks<T extends TopologyNetwork>(nets: readonly T[]): T[] {
  const rank = (n: T) => NETWORK_GROUPS.indexOf(networkGroup(n));
  return nets.toSorted((a, b) => rank(a) - rank(b) || (DRIVER_ORDER[a.driver as NetworkDriver] ?? 6) - (DRIVER_ORDER[b.driver as NetworkDriver] ?? 6) || Number(!!a.stackId) - Number(!!b.stackId) || a.name.localeCompare(b.name, "de"));
}
