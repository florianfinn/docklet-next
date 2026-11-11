<!--
Source: docklet-next/issues/363
Revision: b11b3e70e671dd3c862b966b1d279251842c7319
Retrieved: 2026-11-09
License: Apache-2.0
Deviations: Original release compatibility and supported-layout reference.
-->

# Compatibility

Use the [installation package](install.md) for **linux/amd64**. No ARM image is
part of this release. The hub, agent, served API and UI share one product release
version; the UI uses the hub's version. Private JavaScript package metadata is
not the product version.

## Docker and Compose

Install Docker Engine and the Docker Compose plugin on each host. Generated agent
installers require Compose v2, a POSIX shell, `stat -c`, `awk`, `mktemp` and standard
file utilities (BusyBox is supported). Docklet negotiates the daemon API using its
ping response. The repository does not enforce a product-level minimum Docker
Engine or external Compose release number; API negotiation tests use simulated
API versions and do not prove complete Engine-version compatibility.

In-app updates use the embedded Compose implementation (`compose-go` v2.16.1 and
Docker Compose v5.6.0). These are library versions, not a requirement to install
that external Compose version. Updates require the supported layout described in
[upgrading](upgrade.md#when-the-action-is-unavailable).

## Agent and Edge boundaries

| Boundary | This release |
| --- | --- |
| Agent contract advertised by hub and agent (`AgentHubVersion`) | `18` |
| Minimum accepted agent contract (`MinimumAgentVersion`) | `18` |
| Edge protocol marker | `edge-protocol/1` |
| Minimum Edge marker value | `1` |

Contract numbers are wire-contract versions, not product SemVer. Agents with
contracts older than 18 cannot become compatible hosts; the hub reports
`update-required` and `agent.update-required`. A contract-18 agent must also have
the expected schema fingerprint. Health/version reads and the agent update
recovery routes remain available with authentication. This is the first release
compatibility boundary for both Direct and Edge agents.

Each Edge peer advertises its own protocol marker. A missing, malformed or
duplicate marker counts as protocol 1. This release records the marker without
an Edge protocol refusal path; a future release raising the minimum must add
that enforcement. The agent contract check remains separate.

## Storage, hosting and limits

- SQLite is the only hub database. Database files require local persistent
  storage; network shares and FUSE, including network-backed named volumes, are
  unsupported. Earlier docklet-hub, Arcane, pre-release Docklet Next and
  development PostgreSQL databases or registrations are not imported.
- A dedicated domain or subdomain root is required. Sub-path hosting is
  unsupported for assets, routing, API, WebSockets, authentication and links.
- Official scratch images contain no shell or GPU utilities. The UI shows no GPU
  values with these images.
- In-app hub and agent updates have no automatic rollback. Recover manually using
  the saved env pin and a compatible database backup when needed. Agent updates
  cannot target a version newer than the hub. See [the upgrade guide](upgrade.md).
- Docklet is designed and tested for about 50 containers per host. The
  documented upper bound is 100 containers per host; Docklet does not enforce it,
  and hosts above it run outside the documented range.
- The Dashboard reads at most 500 events of the last 24 hours. When more exist it
  says so and links to the Activity Center, which keeps the full history.
