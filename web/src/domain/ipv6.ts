// IPv6 addresses and prefixes for the network wizard and the connect check.
// Values are bigints; text is accepted in full, compressed or IPv4-embedded
// form and written back canonically (RFC 5952: lower case, longest zero run
// compressed). IPv4 handling stays in networks.ts.

const MAX = (1n << 128n) - 1n;
const HEX_GROUP = /^[0-9a-fA-F]{1,4}$/;

/**
 * Longest subnet prefix the wizard creates: it must hold the gateway and a
 * container besides the unused all-zero address. Docker itself sets no
 * IPv6 prefix limit; /64 is the usual LAN size.
 */
export const SUBNET6_MAX_PREFIX = 126;

function parseV4Tail(text: string): [number, number] | undefined {
  const parts = text.split(".");
  if (parts.length !== 4 || !parts.every((p) => /^\d{1,3}$/.test(p) && Number(p) <= 255)) return undefined;
  const [a = 0, b = 0, c = 0, d = 0] = parts.map(Number);
  return [a * 256 + b, c * 256 + d];
}

/** Address as 128-bit value; zone ids ("%eth0"), brackets and prefixes are refused. */
export function parseIp6(text: string): bigint | undefined {
  const s = text.trim();
  if (!s || s.includes("%") || /[^0-9a-fA-F:.]/.test(s) || s.includes(":::")) return undefined;
  const halves = s.split("::");
  if (halves.length > 2) return undefined;
  const groups = (part: string): number[] | undefined => {
    if (!part) return [];
    const out: number[] = [];
    const items = part.split(":");
    for (let i = 0; i < items.length; i++) {
      const item = items[i] ?? "";
      if (i === items.length - 1 && item.includes(".")) {
        const tail = parseV4Tail(item);
        if (!tail) return undefined;
        out.push(...tail);
      } else if (HEX_GROUP.test(item)) out.push(parseInt(item, 16));
      else return undefined;
    }
    return out;
  };
  const head = groups(halves[0] ?? "");
  const tail = halves.length === 2 ? groups(halves[1] ?? "") : [];
  if (!head || !tail) return undefined;
  // An embedded IPv4 address may only end the address.
  if (halves.length === 2 && (halves[0] ?? "").includes(".")) return undefined;
  const count = head.length + tail.length;
  if (halves.length === 1 ? count !== 8 : count > 7) return undefined;
  const all = [...head, ...Array<number>(8 - count).fill(0), ...tail];
  return all.reduce((n, g) => (n << 16n) | BigInt(g), 0n);
}

/** Canonical text (RFC 5952). */
export function formatIp6(value: bigint): string {
  const groups = Array.from({ length: 8 }, (_, i) => Number((value >> BigInt(112 - i * 16)) & 0xffffn));
  let best = { start: -1, len: 0 };
  for (let i = 0; i < 8; ) {
    if (groups[i] !== 0) {
      i++;
      continue;
    }
    let j = i;
    while (j < 8 && groups[j] === 0) j++;
    if (j - i > best.len && j - i >= 2) best = { start: i, len: j - i };
    i = j;
  }
  const hex = groups.map((g) => g.toString(16));
  if (best.start < 0) return hex.join(":");
  return `${hex.slice(0, best.start).join(":")}::${hex.slice(best.start + best.len).join(":")}`;
}

/** Canonical form of an address text, or undefined when it is no IPv6 address. */
export function canonicalIp6(text: string): string | undefined {
  const v = parseIp6(text);
  return v === undefined ? undefined : formatIp6(v);
}

export interface Cidr6 {
  network: bigint;
  prefix: number;
  size: bigint;
}

export type Cidr6Error = "invalid" | "host-bits";

/** Generic "addr/prefix" (0–128) with the address on the prefix boundary, like Docker refuses host bits. */
export function checkCidr6(text: string): { ok: true; cidr: Cidr6 } | { ok: false; error: Cidr6Error } {
  const [addr = "", bits, ...rest] = text.trim().split("/");
  if (rest.length || bits === undefined || !/^\d{1,3}$/.test(bits)) return { ok: false, error: "invalid" };
  const ip = parseIp6(addr);
  if (ip === undefined) return { ok: false, error: "invalid" };
  const prefix = Number(bits);
  if (prefix > 128) return { ok: false, error: "invalid" };
  const size = 1n << BigInt(128 - prefix);
  if (ip % size !== 0n) return { ok: false, error: "host-bits" };
  return { ok: true, cidr: { network: ip, prefix, size } };
}

const MULTICAST: Cidr6 = { network: 0xffn << 120n, prefix: 8, size: 1n << 120n };
const LINK_LOCAL: Cidr6 = { network: 0xfe80n << 112n, prefix: 64, size: 1n << 64n };
const V4_MAPPED: Cidr6 = { network: 0xffffn << 32n, prefix: 96, size: 1n << 32n };

export type Subnet6Error = "prefix" | "multicast" | "link-local" | "mapped";

/**
 * Subnet the wizard may create. Mirrors Docker's bridge check (moby
 * validateIPv6Subnet: no multicast, no IPv4-mapped address, no overlap with
 * fe80::/64 except that prefix itself), applied to every driver here, plus
 * room for a gateway and a container (SUBNET6_MAX_PREFIX).
 */
export function checkSubnet6(c: Cidr6): Subnet6Error | undefined {
  if (c.prefix > SUBNET6_MAX_PREFIX) return "prefix";
  if (cidr6Overlap(c, MULTICAST)) return "multicast";
  if (inCidr6(c.network, V4_MAPPED)) return "mapped";
  if (cidr6Overlap(c, LINK_LOCAL) && !(c.network === LINK_LOCAL.network && c.prefix === LINK_LOCAL.prefix)) return "link-local";
  return undefined;
}

export function parseCidr6(text: string): Cidr6 | undefined {
  const r = checkCidr6(text);
  return r.ok ? r.cidr : undefined;
}

export function formatCidr6(c: Cidr6): string {
  return `${formatIp6(c.network)}/${c.prefix}`;
}

export function inCidr6(ip: bigint, c: Cidr6): boolean {
  return ip >= c.network && ip < c.network + c.size;
}

export function cidr6Overlap(a: Cidr6, b: Cidr6): boolean {
  return a.network < b.network + b.size && b.network < a.network + a.size;
}

export function cidr6Within(inner: Cidr6, outer: Cidr6): boolean {
  return inner.network >= outer.network && inner.network + inner.size <= outer.network + outer.size;
}

/** Usable container address: inside the prefix but not the subnet-router anycast address (all host bits zero). */
export function usableIn6(ip: bigint, c: Cidr6): boolean {
  return ip > c.network && ip < c.network + c.size && ip <= MAX;
}

/** Candidates scanned for a free pool address; a prefix has far more, the demo never fills that many. */
const SCAN_LIMIT = 4096;

/** True when the pool is known and small enough that `nextFreeIp6` inspects every candidate. */
export function poolFullyScanned6(network: { subnet6?: string; ipRange6?: string }): boolean {
  const subnet = network.subnet6 ? parseCidr6(network.subnet6) : undefined;
  if (!subnet) return false;
  const range = (network.ipRange6 && parseCidr6(network.ipRange6)) || subnet;
  return range.size <= BigInt(SCAN_LIMIT);
}

/** First free address of the range (or prefix), skipping gateway and taken addresses in any notation. */
export function nextFreeIp6(network: { subnet6?: string; gateway6?: string; ipRange6?: string }, taken: readonly string[]): string | undefined {
  const subnet = network.subnet6 ? parseCidr6(network.subnet6) : undefined;
  if (!subnet) return undefined;
  const range = (network.ipRange6 && parseCidr6(network.ipRange6)) || subnet;
  const used = new Set([...taken, network.gateway6 ?? ""].map((x) => parseIp6(x)).filter((x): x is bigint => x !== undefined));
  const end = range.network + range.size;
  for (let ip = range.network, n = 0; ip < end && n < SCAN_LIMIT; ip++, n++) if (usableIn6(ip, subnet) && !used.has(ip)) return formatIp6(ip);
  return undefined;
}
