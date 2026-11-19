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

export interface HubServerCandidate {
  id: string;
  url: string;
  reach: "unknown" | "same-host";
  source: "configured" | "interface" | "published" | "request";
  /** Whether the host is an IP literal or a name; older hubs omit it. */
  kind?: "ip" | "domain";
}

export interface HubDiscovery {
  detectedAt: string;
  /** Present on live hubs; supplied by the server rather than the browser. */
  agentImage?: string;
  /** Server-ranked candidates of live hubs: configured, published, interface, then request addresses. */
  candidates?: HubServerCandidate[];
  /** Live hubs: whether the hub found its own container; "unknown" when Docker could not be inspected. */
  containerDetection?: "container" | "unknown";
  /** The hub's own container; absent when the hub does not run in Docker. */
  container?: {
    name: string;
    /** Known modes drive the demo ranker; live hubs may report other drivers (overlay, custom plugins). */
    networkMode: HubNetworkMode | (string & {});
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

export type CandidateKind = "lan-interface" | "host-address" | "request" | "bridge-local" | "configured" | "interface";
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
const SERVER_KIND: Record<HubServerCandidate["source"], CandidateKind> = { configured: "configured", published: "host-address", interface: "interface", request: "request" };

/** IP literal or name; the server reports it, older hubs are classified from the URL. */
export function serverCandidateKind(candidate: HubServerCandidate): "ip" | "domain" {
  if (candidate.kind) return candidate.kind;
  const host = new URL(candidate.url).hostname;
  return host.startsWith("[") || parseIp(host) !== undefined ? "ip" : "domain";
}

/** The IP candidate agents should use: the first one that other hosts can reach. */
export function preferredIpCandidate(candidates: HubServerCandidate[] | undefined): HubServerCandidate | undefined {
  return candidates?.find((c) => serverCandidateKind(c) === "ip" && c.reach !== "same-host");
}

export function hubAddressCandidates(d: HubDiscovery): HubAddressCandidate[] {
  if (d.candidates) {
    // The internal hub address is an IP address: a name is listed but never recommended.
    const best = preferredIpCandidate(d.candidates);
    return d.candidates.map((candidate) => ({
      id: candidate.id, address: new URL(candidate.url).host, kind: SERVER_KIND[candidate.source],
      reach: candidate.reach, recommended: candidate === best,
    }));
  }
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
