import type { HubDiscovery } from "@/domain/hub-discovery";

import { FIXTURE_NOW } from "./time";

/*
 * Synthetic results of the hub inspecting its own container. Addresses use
 * RFC 5737 ranges only and match the edge hosts' hub address 192.0.2.10:8080.
 */
const at = new Date(FIXTURE_NOW).toISOString();

/** Hub with its own LAN address on an Unraid-style br0 macvlan network plus a local bridge. */
export const DISCOVERY_MACVLAN: HubDiscovery = {
  detectedAt: at,
  container: { name: "docklet-hub", networkMode: "macvlan", listenPort: 8080, publishedPorts: [] },
  interfaces: [
    { network: "br0", driver: "macvlan", address: "192.0.2.10", subnet: "192.0.2.0/24", parent: "br0" },
    { network: "docklet_internal", driver: "bridge", address: "198.51.100.226", subnet: "198.51.100.224/28" },
  ],
  hostAddresses: [],
  requestHost: "docklet.example.com",
  simulated: true,
};

/** Default bridge with a published port; the host address comes from the hub's environment. */
export const DISCOVERY_BRIDGE: HubDiscovery = {
  detectedAt: at,
  container: { name: "docklet-hub", networkMode: "bridge", listenPort: 8080, publishedPorts: [{ hostPort: 8080, containerPort: 8080 }] },
  interfaces: [{ network: "docklet_default", driver: "bridge", address: "198.51.100.242", subnet: "198.51.100.240/28" }],
  hostAddresses: [{ address: "192.0.2.10", source: "configured" }],
  requestHost: "192.0.2.10:8080",
  simulated: true,
};

/** Bridge without a known host address: only the request host hints at the LAN side. */
export const DISCOVERY_BRIDGE_UNKNOWN: HubDiscovery = {
  ...DISCOVERY_BRIDGE,
  hostAddresses: [],
  requestHost: "docklet-hub.example.net:8080",
};

/** Host network: the hub sees the host interfaces directly. */
export const DISCOVERY_HOST: HubDiscovery = {
  detectedAt: at,
  container: { name: "docklet-hub", networkMode: "host", listenPort: 8080, publishedPorts: [] },
  interfaces: [],
  hostAddresses: [
    { address: "127.0.0.1", interface: "lo", source: "host-interface" },
    { address: "192.0.2.10", interface: "eth0", source: "host-interface" },
    { address: "169.254.10.4", interface: "eth1", source: "host-interface" },
  ],
  requestHost: "localhost:8080",
  simulated: true,
};
