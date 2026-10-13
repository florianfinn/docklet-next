import { parse } from "yaml";

import type { NetworkAttachment, Topology, TopologyDependency, TopologyNetwork } from "@/domain/model";
import { canonicalIp6, formatCidr6, nextFreeIp6, parseCidr6 } from "@/domain/ipv6";
import { checkConnect, checkDisconnect, draftFamilies, hasErrors, hasIpv4, nextFreeIp, removeBlock, validateNetworkDraft, type NetworkContainer } from "@/domain/networks";

import type { HubApi, NetworksApi } from "../api";
import { DemoError, wait, type DemoContext } from "./demo-context";
import { containerId, type DemoDb } from "./demo-db";
import { DISCOVERY_MACVLAN } from "./fixtures/hub-discovery";
import { simulateRead, simulationStore } from "./simulation";

/** Compose depends_on in list or map form, as container ids of the same project. */
export function composeDependencies(db: Pick<DemoDb, "stacks" | "compose">): TopologyDependency[] {
  const out: TopologyDependency[] = [];
  for (const stack of db.stacks) {
    let services: Record<string, { depends_on?: unknown }> = {};
    try {
      services = (parse(db.compose[stack.id] ?? "") as { services?: typeof services } | null)?.services ?? {};
    } catch {
      continue;
    }
    for (const [name, svc] of Object.entries(services)) {
      const raw = svc?.depends_on;
      const targets = Array.isArray(raw) ? raw.map(String) : raw && typeof raw === "object" ? Object.keys(raw) : [];
      for (const to of targets) if (to in services) out.push({ from: containerId(stack.id, name), to: containerId(stack.id, to) });
    }
  }
  return out;
}

/** Synthetic block for bridges created without a subnet, like Docker's default address pool. */
function autoSubnet(taken: readonly string[]): { subnet: string; gateway: string } | undefined {
  for (let base = 96; base < 256; base += 16) {
    const subnet = `203.0.113.${base}/28`;
    if (!taken.includes(subnet)) return { subnet, gateway: `203.0.113.${base + 1}` };
  }
  return undefined;
}

/** Synthetic IPv6 pool for prefixes Docker would pick itself; RFC 3849 documentation range only. */
function autoSubnet6(taken: readonly string[]): { subnet6: string; gateway6: string } | undefined {
  for (let n = 0x100; n < 0x200; n++) {
    const subnet6 = `2001:db8:${n.toString(16)}::/64`;
    if (!taken.includes(subnet6)) return { subnet6, gateway6: `2001:db8:${n.toString(16)}::1` };
  }
  return undefined;
}

type ComposeNetworkDef = { external?: unknown; name?: unknown; internal?: unknown };
type ComposeServiceNet = { network_mode?: unknown; networks?: unknown };

/**
 * What `docker compose up` does to the topology after a deploy or restore: the
 * project networks exist (created as bridges when missing, external ones only
 * referenced) and the recreated containers sit exactly in the networks the
 * file names. Runtime connects of the stack's containers are gone, as the
 * connect wizard warns; addresses of unchanged attachments stay.
 */
export function syncStackNetworks(db: DemoDb, stackId: string): void {
  const stack = db.stack(stackId);
  if (!stack) return;
  let doc: { services?: Record<string, ComposeServiceNet | null>; networks?: Record<string, ComposeNetworkDef | null> } | null;
  try {
    doc = parse(db.compose[stackId] ?? "") as typeof doc;
  } catch {
    return;
  }
  const defs = doc?.networks ?? {};
  const networks = [...db.topology.networks];
  const onHost = (name: string) => networks.find((n) => n.hostId === stack.hostId && n.name === name);
  const resolved = new Map<string, TopologyNetwork | undefined>();
  const ensure = (key: string): TopologyNetwork | undefined => {
    if (resolved.has(key)) return resolved.get(key);
    const def = defs[key] ?? {};
    const external = def.external === true;
    const name = typeof def.name === "string" ? def.name : external ? key : `${stackId}_${key}`;
    let n = onHost(name);
    if (!n && !external) {
      const auto = autoSubnet(networks.map((x) => x.subnet ?? ""));
      n = { id: db.nextId(`n-${stack.hostId}`), name, hostId: stack.hostId, driver: "bridge", subnet: auto?.subnet, gateway: auto?.gateway, internal: def.internal === true, stackId };
      networks.push(n);
    }
    resolved.set(key, n);
    return n;
  };

  const own = new Set(stack.services.map((s) => containerId(stackId, s.name)));
  const others = new Set(db.containers().filter((c) => c.stackId !== stackId).map((c) => c.id));
  const ofStack = (id: string) => own.has(id) || (id.startsWith(`c-${stackId}-`) && !others.has(id));
  const previous = db.topology.attachments;
  const attachments = previous.filter((a) => !ofStack(a.containerId));
  for (const svc of stack.services) {
    const cid = containerId(stackId, svc.name);
    const raw = doc?.services?.[svc.name] ?? {};
    const mode = typeof raw.network_mode === "string" ? raw.network_mode : undefined;
    db.isolated.delete(cid);
    if (mode === "none") {
      db.isolated.add(cid);
      continue;
    }
    const wanted: { network: TopologyNetwork | undefined; ip?: string; ip6?: string }[] = mode
      ? [{ network: onHost(mode) }]
      : Array.isArray(raw.networks)
        ? raw.networks.map((k) => ({ network: ensure(String(k)) }))
        : raw.networks && typeof raw.networks === "object"
          ? Object.entries(raw.networks as Record<string, { ipv4_address?: unknown; ipv6_address?: unknown } | null>).map(([k, v]) => ({
              network: ensure(k),
              ip: typeof v?.ipv4_address === "string" ? v.ipv4_address : undefined,
              ip6: typeof v?.ipv6_address === "string" ? canonicalIp6(v.ipv6_address) : undefined,
            }))
          : [{ network: ensure("default") }];
    for (const { network: n, ip: fixed, ip6: fixed6 } of wanted) {
      if (!n || attachments.some((a) => a.containerId === cid && a.networkId === n.id)) continue;
      const taken = attachments.filter((a) => a.networkId === n.id && a.ip).map((a) => a.ip as string);
      const before = previous.find((a) => a.containerId === cid && a.networkId === n.id);
      const kept = before?.ip;
      const ip = n.driver === "host" || !hasIpv4(n) ? undefined : (fixed ?? (kept && !taken.includes(kept) ? kept : nextFreeIp(n, taken)));
      let ip6: string | undefined;
      if (n.ipv6) {
        const taken6 = attachments.filter((a) => a.networkId === n.id && a.ip6).map((a) => a.ip6 as string);
        ip6 = fixed6 ?? (before?.ip6 && !taken6.includes(before.ip6) ? before.ip6 : nextFreeIp6(n, taken6));
      }
      attachments.push({ containerId: cid, networkId: n.id, ip, staticIp: fixed ? true : undefined, ip6, staticIp6: fixed6 ? true : undefined });
    }
  }
  db.topology = { ...db.topology, networks, attachments };
}

export function createNetworksApi(ctx: DemoContext): NetworksApi & HubApi {
  const { db } = ctx;
  const snapshot = (): Topology => ({ ...structuredClone(db.topology), dependencies: composeDependencies(db) });

  function network(id: string): TopologyNetwork {
    const n = db.topology.networks.find((x) => x.id === id);
    if (!n) throw new DemoError("not-found");
    return n;
  }

  function container(id: string): NetworkContainer {
    const c = db.containers().find((x) => x.id === id);
    if (!c) throw new DemoError("not-found");
    return c;
  }

  function online(hostId: string): void {
    if (db.host(hostId)?.status !== "online") throw new DemoError("host-offline");
  }

  function commit(event: string, hostId: string): void {
    db.pushEvent({ kind: "config", level: "info", title: event, hostId });
    db.changed("topology", "containers");
  }

  return {
    getTopology: () => simulateRead("networks", () => (simulationStore.get().empty ? { networks: [], attachments: [], dependencies: [] } : snapshot())),

    async createNetwork(input) {
      await wait(300);
      online(input.hostId);
      const onHost = db.topology.networks.filter((n) => n.hostId === input.hostId);
      // The adapter checks the draft again instead of trusting the wizard.
      const errors = validateNetworkDraft(input, onHost);
      if (errors.name === "exists") throw new DemoError("exists");
      if (hasErrors(errors)) throw new DemoError("invalid-network");
      const name = input.name.trim();
      const families = draftFamilies(input);
      const configured = families.ipv4 && !!input.subnet.trim();
      const auto = families.ipv4 && !configured ? autoSubnet(db.topology.networks.map((n) => n.subnet ?? "")) : undefined;
      const prefix6 = families.ipv6 && input.subnet6?.trim() ? parseCidr6(input.subnet6) : undefined;
      const auto6 = families.ipv6 && !prefix6 ? autoSubnet6(db.topology.networks.map((n) => n.subnet6 ?? "")) : undefined;
      const created: TopologyNetwork = {
        id: db.nextId(`n-${input.hostId}`),
        name,
        hostId: input.hostId,
        driver: input.driver,
        subnet: configured ? input.subnet.trim() : auto?.subnet,
        gateway: families.ipv4 ? input.gateway.trim() || auto?.gateway : undefined,
        ipRange: (families.ipv4 && input.ipRange.trim()) || undefined,
        parent: input.driver === "bridge" ? undefined : input.parent.trim(),
        subnetConfigured: configured,
        ipv4: families.ipv4 ? undefined : false,
        ...(families.ipv6
          ? {
              ipv6: true,
              // Stored canonically so overlap and duplicate checks compare like with like.
              subnet6: prefix6 ? formatCidr6(prefix6) : auto6?.subnet6,
              gateway6: canonicalIp6(input.gateway6 ?? "") ?? auto6?.gateway6,
              ipRange6: input.ipRange6?.trim() ? formatCidr6(parseCidr6(input.ipRange6)!) : undefined,
              subnet6Configured: !!prefix6,
            }
          : {}),
        internal: input.internal,
      };
      db.topology = { ...db.topology, networks: [...db.topology.networks, created] };
      commit(`Netzwerk ${name} (${input.driver}) angelegt`, input.hostId);
      return structuredClone(created);
    },

    async removeNetwork(id) {
      await wait(300);
      const n = network(id);
      online(n.hostId);
      const block = removeBlock(db.topology, n);
      if (block) throw new DemoError(block);
      db.topology = { ...db.topology, networks: db.topology.networks.filter((x) => x.id !== id) };
      commit(`Netzwerk ${n.name} entfernt`, n.hostId);
    },

    async connectNetwork(input) {
      await wait(300);
      const n = network(input.networkId);
      const c = container(input.containerId);
      online(n.hostId);
      const check = checkConnect(db.topology, c, n, input.ip ?? "", input.ip6 ?? "");
      if (check.block) throw new DemoError(check.block);
      if (check.ipError || check.ip6Error) throw new DemoError("invalid-ip");
      const attachment: NetworkAttachment = { containerId: c.id, networkId: n.id, ip: check.ip, staticIp: input.ip?.trim() ? true : undefined, ip6: check.ip6, staticIp6: input.ip6?.trim() ? true : undefined };
      db.topology = { ...db.topology, attachments: [...db.topology.attachments, attachment] };
      db.isolated.delete(c.id);
      const addresses = [check.ip, check.ip6].filter(Boolean).join(", ");
      commit(`${c.name} mit ${n.name} verbunden${addresses ? ` (${addresses})` : ""}`, n.hostId);
      return structuredClone(attachment);
    },

    async disconnectNetwork({ networkId, containerId: cid }) {
      await wait(300);
      const n = network(networkId);
      const c = container(cid);
      online(n.hostId);
      const check = checkDisconnect(snapshot(), c, n);
      if (check.block) throw new DemoError(check.block);
      const attachments = db.topology.attachments.filter((a) => !(a.containerId === cid && a.networkId === networkId));
      db.topology = { ...db.topology, attachments };
      if (!attachments.some((a) => a.containerId === cid)) db.isolated.add(cid);
      commit(`${c.name} von ${n.name} getrennt`, n.hostId);
    },

    discoverHub: () => simulateRead("settings", () => structuredClone(DISCOVERY_MACVLAN)),
  };
}
