# Network management

The network service validates writes on the Docker host, including Direct and
Edge agents. Network writes keep handler activities and audit events; they do
not enter the runtime run gate. The HTTP adapter maps the wire IPAM configuration
without changing pool order, labels or `options.parent` used by topology.

## Operations, rights and transport

All paths below are relative to `/api/environments/{id}/networks`. Direct
transport forwards HTTP to the agent. Edge forwards the same method, path and
body through the named command; the agent executes the same service validation.

| Operation ID | Method and suffix | Required right | Edge command |
| --- | --- | --- | --- |
| list-networks | GET (root) | networks:list | network.list |
| network-counts | GET /counts | networks:list | network.counts |
| create-network | POST (root) | networks:create | network.create |
| get-network-topology | GET /topology | networks:read | network.topology |
| get-network | GET /{networkId} | networks:read | network.inspect |
| delete-network | DELETE /{networkId} | networks:delete | network.delete |
| prune-networks | POST /prune | networks:prune | network.prune |
| connect-network-container | POST /{networkId}/connect | networks:connect | network.connect |
| disconnect-network-container | POST /{networkId}/disconnect | networks:disconnect | network.disconnect |

## Create validation

`enableIPv4` is optional: omission preserves the daemon default (IPv4 on).
`enableIPv6` enables IPv6; disabling IPv4 requires IPv6. IPv6-only is supported.
Drivers are bridge, macvlan and ipvlan; an omitted driver means bridge. Internal
networks require bridge. LAN drivers require a parent interface name, a subnet
and gateway for each enabled family. Interface names use ASCII letters, digits,
underscore, dot, colon and hyphen, start with a letter or digit, and have at most
15 characters. The daemon validates parent existence.

Names use ASCII letters, digits, underscore, dot and hyphen, start with a letter
or digit, and have at most 63 characters. The reserved names are bridge, host,
none, default and ingress, case insensitive. Duplicate names on this host return
`network.exists` (409). Unsupported drivers, invalid names, reserved names,
missing/invalid parents, invalid internal/family combinations, empty label or
option keys and invalid IPAM return `network.invalid-network` (400).

There is at most one IPAM pool per enabled family. Each subnet must be a
canonical CIDR network address. Gateway and auxiliary addresses must have the
same family and lie inside it; gateways cannot be the subnet address or IPv4
broadcast. Auxiliary names must be nonempty and unique, and their values cannot
equal the gateway. IP ranges must be canonical CIDRs wholly inside the subnet.
A pool for a disabled family is rejected. IPAM driver selection and IPAM options
are excluded; the default IPAM driver is used. Overlap with any IPAM subnet on
this host, comparing each family separately, returns `network.overlap` (409).
This includes LAN networks on different parents. Other hosts are independent.

## Membership validation

Missing networks or containers return `network.not-found` (404). Connect rejects
host/none networks with `network.builtin-network` (409), and containers in host,
none or container-sharing mode with `network.network-mode` (409). Existing
membership returns `network.already-connected` (409). Membership is read from
container NetworkSettings, including stopped containers.

Fixed addresses must match their field family and a user-configured IPAM subnet
of that family. They must lie inside that subnet and cannot be its network
address, IPv4 broadcast, gateway or auxiliary reservation. An address used by any
other endpoint, including a stopped container's IPAMConfig reservation, is
rejected. These failures return `network.invalid-ip` (400). IPv6 has no broadcast.
Aliases must be nonempty DNS labels (ASCII letters, digits and internal hyphens,
1–63 characters); duplicates are removed. Invalid aliases return
`network.invalid-network` (400). Disconnect requires membership, otherwise
`network.not-connected` (409), and rejects host/none with `network.network-mode`.

Compose and Unraid container memberships may be changed at runtime. The UI warns
that recreation restores the owning definition. This ownership does not block
connect or disconnect.

## Delete and prune

Delete rejects built-ins with `network.builtin-network` (409). The shared
`docker.IsDefaultNetwork` predicate is authoritative for delete, counts and sort:
bridge, host, none and ingress. The reserved create name default is not a built-in.
Compose ownership follows the existing Compose project-label convention:
a nonempty `com.docker.compose.project` or `com.docker.compose.network` label
protects deletion with `network.compose-managed` (409). References from any
container, including stopped containers, return `network.in-use` (409).
External-network references from stack files are not detected.

Prune retains daemon behaviour and the scheduled job. Interactive prune requires
an acting user and attributes the activity and audit event to that user. M9 has
no prune UI; unused networks are removed individually. Per-network usage details
are exposed in the table by an icon with hover and focus detail.

## Compatibility and limits

Agent contract v6 and minimum agent version 6 cover the network write surface,
including connect/disconnect Edge commands. Hub and agents must upgrade together;
older agents are unsupported. The Edge tunnel protocol remains tunnel/v1.

IPAM driver choice, multiple pools per family, attachable, Swarm/overlay and a
network prune UI are out of scope. Validation is a snapshot before the daemon
write; concurrent external changes remain subject to daemon checks. The daemon
chooses unspecified bridge pools and validates platform-specific driver options.
