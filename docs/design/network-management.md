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
broadcast. Auxiliary names must be nonempty and unique. Their values must be
unique within each family, usable inside the subnet and different from the
gateway. Network addresses and IPv4 broadcast addresses cannot be reserved.
IP ranges must be canonical CIDRs wholly inside the subnet.
A pool for a disabled family is rejected. IPAM driver selection and IPAM options
are excluded; the default IPAM driver is used. Overlap with any IPAM subnet on
this host, comparing each family separately, returns `network.overlap` (409).
This includes LAN networks on different parents. Other hosts are independent.

## Membership validation

Empty or whitespace-only network/container IDs return `network.invalid-network`
(400). A container that disappears during the unrelated reference scan is skipped.
A target container must still exist. Missing networks or containers return `network.not-found` (404). Connect rejects
host/none networks with `network.builtin-network` (409), and containers in host,
none or container-sharing mode with `network.network-mode` (409). Existing
membership returns `network.already-connected` (409). Membership is read from
container NetworkSettings, including stopped containers.

Fixed addresses must match their field family and be usable in any configured
IPAM subnet of that family, including Docker-assigned pools. Inspect does not
distinguish the origin of these pools; Docker may refuse static addresses on an
auto-assigned pool. On existing networks, every pool of the family is considered;
network creation remains limited to one per family. An address cannot be the containing subnet's network
address, IPv4 broadcast, gateway or auxiliary reservation. An address used by any
other endpoint, including a stopped container's IPAMConfig reservation, is
rejected. These failures return `network.invalid-ip` (400). IPv6 has no broadcast. Docker 403/409 removal conflicts map to `network.in-use`
(409), and recognized connect address conflicts map to `network.invalid-ip` (400).
Recognized duplicate-endpoint or already-attached connect conflicts return
`network.already-connected` (409). Docker shares status codes across these
conditions; unknown connect 403/409 errors remain generic server errors without
raw daemon text.
Aliases must be nonempty DNS labels (ASCII letters, digits and internal hyphens,
1–63 characters); the web trims and lowercases aliases before sending and
removes duplicates case insensitively. The server also removes case-insensitive
duplicates during validation. Invalid aliases return
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

Scheduled prune keeps its existing selection and removal semantics: it selects
local, non-built-in, non-ingress networks whose running endpoints are prune
targets, then removes networks with no running endpoints after the targets have
been processed. Compose labels and stopped-container references do not protect
these networks from scheduled prune. It can therefore remove a network used by a
stopped container; individual delete remains protected. Its internal removal
entry bypasses the user-facing guards after selection/recheck, while retaining
Docker's final checks and audit events. Recovery uses the same removal entry.
Interactive network prune and system prune retain native Docker prune semantics. Interactive prune requires
an acting user and attributes the activity and audit event to that user. M9 has
no prune UI; unused networks are removed individually. Per-network usage details
are exposed in the table by an icon with hover and focus detail.

## Read identity and incomplete usage

The network list supplies row identities and list metadata, including built-in
and Compose ownership. Topology/detail reads enrich those rows; their failure
cannot erase a listed network or invent an unused network. A failing list keeps
previously observed rows for that host, while hosts with successful lists keep
their fresh rows. Without a prior successful list there are no retained rows.
An online host reporting an inventory read or mapping failure with provably fresh
list data and failed detail reads shows one network-specific partial notice,
including hosts not selected in the picker. Agent availability, credential and
version failures keep the ordinary inventory reason and snapshot wording.
Freshness evidence is a host observation timestamp or a row whose details and
inventory are fresh. All details may fail while a remote list still supplies a
host timestamp. A failed list keeps the ordinary inventory notice: retained
rows keep their “as of” timestamp; without a snapshot it says no retained values
are available yet. The picker never labels such a list failure as partial.

Local responses can lack observation timestamps. When all details fail without
a host timestamp or fresh row, the current domain observations cannot distinguish
a fresh list from a failed list with retained rows. This ambiguous case uses the
ordinary inventory notice conservatively, including its existing timestamp when
available, otherwise the no-snapshot wording. It never claims a partial read.
This presentation limit does not relax any unknown-usage write guard.

Unavailable details or stale rows make membership and address use unknown.
Delete, connect, disconnect and address hints remain blocked until usage can be
read again. Known built-in and Compose deletion protections take precedence over
the weaker detail-unavailable explanation, and their usage indicators preserve
that ownership without claiming zero attachments. Stopped containers remain part
of membership and taken-address checks.

## Compatibility and limits

Agent contract v6 and minimum agent version 6 cover the network write surface,
including connect/disconnect Edge commands. Hub and agents must upgrade together;
older agents are unsupported. The Edge tunnel protocol remains tunnel/v1.

IPAM driver choice, multiple pools per family, attachable, Swarm/overlay and a
network prune UI are out of scope. Validation is a snapshot before the daemon
write; concurrent external changes remain subject to daemon checks. IPv4 /31 and /32 and IPv6 /128 have no usable gateway or fixed address under
this policy and are rejected on create, even without an explicit gateway. IPv6
/127 has one usable address (the last); reserving it as gateway or auxiliary
address leaves no fixed endpoint address. Existing small pools return a clear
`network.invalid-ip` if no usable unreserved address matches. Auxiliary names are
unique by the map-shaped wire contract. Web and server both reject duplicate
auxiliary values, subnet addresses and IPv4 broadcast addresses. IPv6 has no
broadcast address; its last address may be reserved when otherwise usable.
The default Docker IPAM driver reserves subnet addresses (and IPv4 broadcast)
and allocates each auxiliary address separately, rejecting an already allocated
value. These rules are verified against the
[Moby allocator](https://github.com/moby/moby/blob/b711d21792db6db488de976eb3c4a653c92c3bc3/daemon/libnetwork/ipams/defaultipam/allocator.go)
and [auxiliary allocation](https://github.com/moby/moby/blob/b711d21792db6db488de976eb3c4a653c92c3bc3/daemon/libnetwork/network.go)
on 2026-10-24. The daemon chooses unspecified bridge pools and validates
platform-specific driver options.
