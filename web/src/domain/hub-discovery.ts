// What the hub reports about its own reachability. The browser cannot see
// Docker addresses, so the hub inspects its container and network setup on
// the server and the UI only ranks and explains the result.

import { cidrOverlap, parseCidr, parseIp } from "./networks";

export type HubNetworkMode = "bridge" | "host" | "macvlan" | "ipvlan";

/** One interface of the hub container, as Docker reports it per network. */
export interface HubInterface {
  network: string;
  driver: "bridge" | "macvlan" | "ipvlan";
  address: string;
  /** Subnet of that Docker network. */
  subnet?: string;
  parent?: string;
}

export interface HubDiscovery {
  detectedAt: string;
  /** The hub's own container; absent when the hub does not run in Docker. */
  container?: {
    name: string;
    networkMode: HubNetworkMode;
    /** Port the hub listens on inside its container. */
    listenPort: number;
    publishedPorts: { hostPort: number; containerPort: number; hostIp?: string }[];
  };
  interfaces: HubInterface[];
  /**
   * LAN addresses of the Docker host. Visible to the hub only with the host
   * network or when the operator sets them in its environment.
   */
  hostAddresses: { address: string; interface?: string; source: "host-interface" | "configured" }[];
  /** Host header of the request that loaded the page, as the server received it. */
  requestHost?: string;
  simulated: boolean;
}

export type CandidateKind = "lan-interface" | "host-address" | "request" | "bridge-local";
/** Who can reach the address: other machines, only processes on the same host, or not knowable. */
export type CandidateReach = "lan" | "same-host" | "unknown";

export interface HubAddressCandidate {
  /** Stable within one discovery; stored as the origin of a prefilled value. */
  id: string;
  /** "host:port" as the internal hub address field expects it. */
  address: string;
  kind: CandidateKind;
  reach: CandidateReach;
  /** Docker network or host interface the address belongs to. */
  via?: string;
  driver?: HubInterface["driver"];
  recommended: boolean;
}

const LOOPBACK = parseCidr("127.0.0.0/8");
const LINK_LOCAL = parseCidr("169.254.0.0/16");

function routable(address: string): boolean {
  const ip = parseIp(address);
  if (ip === undefined) return true;
  const one = { network: ip, prefix: 32, size: 1 };
  return !(LOOPBACK && cidrOverlap(one, LOOPBACK)) && !(LINK_LOCAL && cidrOverlap(one, LINK_LOCAL));
}

function splitHost(hostHeader: string): { host: string; port?: number } {
  const m = /^(\[[^\]]+\]|[^:]+)(?::(\d+))?$/.exec(hostHeader.trim());
  return m ? { host: m[1] ?? "", port: m[2] ? Number(m[2]) : undefined } : { host: hostHeader };
}

/**
 * Address candidates, best first. LAN interfaces of macvlan/ipvlan networks
 * and host addresses are reachable from other hosts; bridge addresses only
 * from the same host and are never recommended. The request host is a hint
 * of how the browser reached the hub, never a container address.
 */
export function hubAddressCandidates(d: HubDiscovery): HubAddressCandidate[] {
  const out: Omit<HubAddressCandidate, "recommended">[] = [];
  const c = d.container;
  const listen = c?.listenPort ?? 8080;
  const published = c?.publishedPorts.find((p) => p.containerPort === listen && (!p.hostIp || p.hostIp === "0.0.0.0"));

  for (const i of d.interfaces) {
    if (i.driver !== "bridge" && routable(i.address)) out.push({ id: `if:${i.network}`, address: `${i.address}:${listen}`, kind: "lan-interface", reach: "lan", via: i.network, driver: i.driver });
  }
  // Host addresses work with the host network directly, with a bridge only through a published port.
  const hostPort = c?.networkMode === "host" || !c ? listen : published?.hostPort;
  if (hostPort) {
    for (const h of d.hostAddresses) {
      if (routable(h.address)) out.push({ id: `host:${h.address}`, address: `${h.address}:${hostPort}`, kind: "host-address", reach: "lan", via: h.interface });
    }
  }
  if (d.requestHost) {
    const { host, port } = splitHost(d.requestHost);
    const local = host === "localhost" || !routable(host);
    const known = out.some((o) => o.address.startsWith(`${host}:`));
    if (!local && !known) out.push({ id: `request:${host}`, address: port ? `${host}:${port}` : host, kind: "request", reach: "unknown" });
  }
  for (const i of d.interfaces) {
    if (i.driver === "bridge") out.push({ id: `if:${i.network}`, address: `${i.address}:${listen}`, kind: "bridge-local", reach: "same-host", via: i.network, driver: "bridge" });
  }

  const best = out.find((o) => o.reach === "lan") ?? out.find((o) => o.reach === "unknown");
  return out.map((o) => ({ ...o, recommended: o === best }));
}
