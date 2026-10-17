# Edge tunnel and agent mode threat review

This review covers the host connection adopted from Arcane for #80: direct agents
reached by the hub, Edge agents that dial out to the hub, and the agent HTTP
surface listed in [agent surface](agent-surface.md). It assumes the product scale
of one operator and a few private users on one hub. It does not design for
multiple hub processes, failover or hostile co-tenants. Arcane's behaviour is the
baseline; anything beyond it is a decision at the end of this record.

File references are relative to `backend/` unless they start with `docs/`, `web/`
or `types/`.

## Assets

- **Docker on each host.** The agent mounts the Docker socket. Control of the
  agent API is root-equivalent control of that host.
- **Agent token.** One random secret per host. The same value authenticates the
  hub to the agent, the Edge agent to the hub, and acts as an environment-scoped
  hub API key. The hub stores it in plain text in `environments.access_token`
  (`resources/migrations/sqlite/001_baseline.sql`); the `api_keys` row stores
  only an Argon2 hash of the same key (`internal/apikey/service.go`).
- **Data in transit to a host.** Compose files, `.env` contents, uploads, shell
  and log streams, and registry credentials that the hub pushes to every
  reachable agent (`internal/environment/service.go`, health check and
  `SyncRegistriesToEnvironment`).
- **Hub browser origin.** Admin sessions on the hub origin can command every
  host. Content served on that origin is trusted by the browser.
- **Hub database and backups.** They hold all agent tokens; hub archives are
  unencrypted by design ([hub backup](hub-backup.md)).

## Trust boundaries

1. Browser to hub: user identity per [identity](identity.md). The hub checks
   user permissions before it forwards anything to an agent.
2. Hub to direct agent: HTTP(S) to the host's published agent port, token in
   `X-Arcane-Agent-Token`, `X-API-Key` and `Authorization: Bearer`
   (`pkg/remenv/client.go`, `ApplyAgentTokenHeaders`).
3. Edge agent to hub: outbound WebSocket, gRPC or poll to `/api/tunnel/*` on the
   hub listener, token in headers or the first register message
   (`pkg/libarcane/edge/server.go`, `HandleConnect`, `authStreamInterceptorInternal`,
   `pkg/libarcane/edge/poll_control.go`, `HandlePoll`). Hub requests then travel
   back through the tunnel and still carry the token; the agent replays them into
   its own handler (`pkg/libarcane/edge/client.go`, `handleRequest`).
4. Agent to Docker: the socket, guarded only by the agent's token check
   (`internal/auth/middleware_echo.go`, `agentAuth`, constant-time
   `AgentTokenMatches`).

The Edge agent still runs its HTTP server inside its container; Edge onboarding
publishes no port for it, so only peers on the same container network can reach
it, and they still need the token.

## Attacker positions

- **N: network attacker** between hub and host, or between host and hub.
- **T: holder of a stolen agent token**, for example from shell history, a copied
  compose file or a leaked `.env`.
- **H: compromised agent host** with root on that host.
- **M: compromised hub** process, database or backup archive.

## Threats

Verdict "accepted" means Arcane's behaviour is adopted, with at most onboarding
text or generator rules from [host onboarding](host-onboarding.md). "Needs change"
means M5 needs code beyond the adopted behaviour.

### T1. Eavesdropping on a direct connection (N)

The hub sends the token with every request to the direct agent. Arcane supports
an agent TLS listener (`TLS_ENABLED`, `TLS_CERT_FILE`, `TLS_KEY_FILE` in
`internal/config/config.go`) and the hub verifies HTTPS with Go's default
verifier. Arcane's snippets default to plain HTTP and publish the agent port on
every host interface (`internal/environment/children/snippets/service.go`,
`Direct`).

Residual risk: on plain HTTP a passive observer on the path learns the token and
gains T. Acceptable on a trusted LAN or VPN, which is the intended direct use.
Verdict: **accepted**; onboarding warns when the agent address is plain HTTP and
recommends binding the published port to the address the hub uses (D5).

### T2. Eavesdropping or impersonating the hub on an Edge connection (N)

The Edge agent dials `MANAGER_API_URL`. HTTPS and WSS use system roots or
`EDGE_MTLS_CA_FILE` and `EDGE_MTLS_SERVER_NAME`
(`buildManagerClientTLSConfigInternal` in `pkg/libarcane/edge/tls.go`). Plain
`http://` and `ws://` remain allowed; gRPC then uses insecure credentials
(`pkg/libarcane/edge/client_transport_grpc.go`).

Residual risk: with a plain URL, N reads the token and every tunneled request,
and can pose as the hub to the agent. Verdict: **accepted** with the same warning
as T1 for non-HTTPS hub URLs (D5).

### T3. Stolen token gives control of its host (T)

Direct mode: anyone who reaches the agent port with the token is treated as the
built-in agent superuser (`agentSudoInternal` in
`internal/auth/middleware_echo.go`). Both modes: the token is also an
environment bootstrap API key with every permission scoped to that environment
(`CreateEnvironmentApiKey` in `internal/apikey/service.go`) and is accepted by
the hub in `X-Arcane-Agent-Token` (`managerAuth`). A token holder can therefore
drive the host through the hub API wherever the hub is reachable, even though an
Edge host exposes no port.

Mitigations in Arcane: tokens are long random values compared in constant time;
rotation replaces the key, sets the host to pending and deletes the old key
(`RegenerateEnvironmentApiKey`, `linkEnvironmentApiKey` in
`internal/environment/service.go`); deleting a host cascades its key.

Residual risk: the token equals root on one host, in both modes. The scope stays
one host. Verdict: **accepted**. Onboarding keeps the token out of command-line
arguments and in a mode 600 `.env` (T10) and offers rotation.

### T4. Stolen token impersonates an Edge agent (T)

With the token, an attacker can register a tunnel for the host. The registry
keeps one active session per environment and replaces it only when the old
session is closed, stale for `TunnelStaleTimeout` (2 minutes) or reports the same
agent instance ID (`RegisterSession` in `pkg/libarcane/edge/registry.go`). While
the real agent is connected, a second instance is rejected. When it is offline,
the attacker wins and then receives forwarded user requests, uploads, `.env`
contents and the periodic registry credential sync. It can also inject Edge
events, which the hub turns into events and notifications
(`handleEvent` in `server.go`, `handleEdgeEventInternal` in
`internal/bootstrap/edge_bootstrap.go`).

Generated Edge mTLS does not stop this: `/api/tunnel/mtls/enroll` authenticates
only the token and re-serves cached client assets during the 15-minute cooldown
(`HandleMTLSEnroll`, `managerMTLSReenrollCooldown` in `tls.go`). Only certificates
provisioned outside the hub with `EDGE_MTLS_MODE=required` bind a session to more
than the token. Connect and disconnect transitions are recorded as events
(`createEdgeConnectionEvent`).

Residual risk: equal to T3 plus disclosure of what the hub sends to that host.
Verdict: **accepted**; the Hosts page shows connection events and the connected
agent instance, and rotation ends the impersonation (D4 on mTLS).

### T5. Compromised host reaches beyond itself (H)

H owns its host already. Beyond it, H can read the token (T3, T4 for this host
only), receive every enabled registry credential of the hub, because the sync
sends the full set to each agent (`GetEnabledRegistryCredentials`,
`SyncRegistriesToEnvironment` in `internal/environment/service.go`), report false
inventory, and emit misleading events. It cannot use its token for another host:
hub grants come from `EnvironmentPermissionSet` (`pkg/authz/permission_set.go`)
and tunnel registration resolves one environment ID per token.

Residual risk: registry credentials leak to every host, and that host's own view
is untrustworthy. With one operator who uses the same registries everywhere this
matches the operator's own trust in each host. Verdict: **accepted**; the
[agent surface](agent-surface.md) row for `registry` already records it.

### T6. Malicious agent responses reach the browser on the hub origin (H, T)

The hub copies the agent's response headers to the browser, removing only
hop-by-hop headers: `ProxyHTTPRequest` in `pkg/libarcane/edge/proxy.go` for Edge
and `writeProxyResponse` with `CopyResponseHeaders` in
`internal/middleware/environment.go` for direct hosts. No global
`Content-Security-Policy` or `X-Content-Type-Options` header is set for proxied
responses. A malicious agent can therefore return `Set-Cookie` for the hub origin
or HTML/script with an active content type on an environment path. If an admin
opens such a path, for example a download link, script runs with the admin's
session and can command every host. Body size is bounded by the tunnel frame
limit of 16 MiB per gRPC message and 32 MiB per WebSocket frame (`tunnel.go`,
`server.go` `MaxRecvMsgSize`) and by proxy timeouts.

Residual risk: one compromised host or token escalates to the hub and all hosts.
Verdict: **needs change** (D2): filter proxied response headers.

### T7. Compromised hub or leaked hub backup (M)

The hub is the trust anchor. It holds all tokens in plain text and may call every
allowlisted agent module, including agent self-upgrade
([agent surface](agent-surface.md), `system`). A leaked database file or hub
archive yields every agent token without the external `ENCRYPTION_KEY`, unlike
registry credentials, which are stored encrypted.

Residual risk: hub compromise is compromise of all hosts; an archive leak is the
same for anyone who can reach an agent port or the hub API. Verdict:
**accepted** for hub compromise; token storage at rest is D3. Hub archives must
be protected like host root credentials.

### T8. Incompatible or downgraded agent (N, T, H)

Arcane enforces no agent version. The hub publishes `agentContractVersion`,
`agentContractFingerprint` and `minimumAgentContractVersion` in its version info
(`internal/version/service.go`, `types/contract/agent.go`: both versions are 2),
and an agent returns the same structure on `/api/app-version`. No code compares
them; `GetEnvironmentVersion` in `internal/environment/handler.go` only reads
and returns the value. The Edge register message carries token, instance ID,
capabilities and resume session, but no version (`proto/tunnel/v1/tunnel.proto`,
`RegisterRequest`); capabilities are feature flags such as `proto-parity-v1` and
`chunked-request` (`pkg/libarcane/edge/command_client.go`). Protocol size limits
are enforced on both sides as listed under T6, and stale sessions are reaped every
minute (`StartCleanupLoop`).

Residual risk: an old agent may lack checks that the hub relies on, or silently
misread changed payloads. #81 requires enforcement. Verdict: **needs change**
(D1).

### T9. Resource exhaustion on tunnel endpoints (N)

`/api/tunnel/connect` is rate limited per IP (60 per minute, burst 30) and per
token (10, burst 3); `/api/tunnel/mtls/enroll` per IP and per token (10, burst 3);
`/api/tunnel/poll` has no rate limit (`registerEdgeTunnelRoutes` in
`internal/bootstrap/edge_bootstrap.go`). Valid tokens are cached; misses query
SQLite (`ResolveEdgeEnvironmentByToken`). Message sizes and stream delivery
(`streamDeliveryTimeout`, 5 seconds) are bounded.

Residual risk: an unauthenticated flood on the poll endpoint costs one indexed
lookup per request. At this scale a reverse proxy or firewall in front of the hub
is the expected control. Verdict: **accepted**.

### T10. Token exposure during onboarding (T)

Arcane's snippets embed the token as `-e AGENT_TOKEN=<token>` in a `docker run`
command and inline in compose (`snippets/service.go`). That places it in shell
history, process listings and copied compose files. Arcane also accepts the token
from a file (`AGENT_TOKEN` with `options:"file"` in `internal/config/config.go`).
The frozen reference writes the token only into `.env` with mode 600
(`web/src/domain/agent-install.ts`, `agentScript`).

Residual risk: the token stays visible in `docker inspect` to anyone with Docker
access on that host, who has root there already. The copyable commands write
`.env` through a heredoc, so a pasted token can stay in interactive shell
history; the downloadable package avoids that. Verdict: **accepted** with the
generator rules in [host onboarding](host-onboarding.md).

## Summary

| Threat | Position | Verdict |
| --- | --- | --- |
| T1 Direct eavesdropping | N | accepted |
| T2 Edge eavesdropping or hub impersonation | N | accepted |
| T3 Stolen token controls its host | T | accepted |
| T4 Stolen token impersonates Edge agent | T | accepted |
| T5 Compromised host reaches beyond itself | H | accepted |
| T6 Agent responses on hub origin | H, T | needs change |
| T7 Compromised hub or backup | M | accepted |
| T8 Incompatible or downgraded agent | N, T, H | needs change |
| T9 Tunnel endpoint exhaustion | N | accepted |
| T10 Token exposure during onboarding | T | accepted |

## Decisions

The maintainer chose option (a), the recommendation, for every item below on 2026-10-17.

D1. **Agent version enforcement (T8).**
Recommended: (a) after a direct health check or Edge registration, the hub reads
the agent's `/api/app-version`; below `MinimumAgentVersion` or with a different
fingerprint at the same contract version, the host becomes "update required"
and the hub forwards only health, version and agent self-upgrade. The Edge
protocol is versioned with the agent contract; `tunnel.proto` stays unchanged.
(b) as (a), plus a version field in `RegisterRequest` so the hub can reject an
Edge session before it is accepted; this changes the tunnel protocol.
(c) show a warning only and keep forwarding.

D2. **Proxied agent response headers (T6).**
Recommended: (a) forward an allowlist of `Content-Type`, `Content-Length`,
`Content-Disposition`, `Content-Encoding`, `Content-Range`, `Accept-Ranges`,
`Cache-Control`, `ETag` and `Last-Modified`, always add
`X-Content-Type-Options: nosniff`, never forward `Set-Cookie`. One shared filter
covers direct and Edge forwarding. (b) drop only `Set-Cookie` and
security-policy headers. (c) keep Arcane's behaviour.

D3. **Agent token at rest (T7).**
Recommended: (a) keep Arcane's plain-text column for M5 and state in the hub
backup and onboarding texts that archives contain host credentials.
(b) store the token encrypted with `ENCRYPTION_KEY` plus a keyed hash for lookup;
this needs a new migration and token handling in the tunnel resolver.

D4. **Edge mTLS (T4).**
Recommended: (a) keep Arcane's mTLS modes as an advanced, environment-only
setting and do not offer generated mTLS in onboarding, since token enrollment
adds little against a token thief. (b) offer generated mTLS in onboarding.
(c) remove mTLS code and settings.

D5. **Plain HTTP connections (T1, T2).**
Recommended: (a) allow `http://` agent addresses and hub URLs, with an onboarding
warning that the network must be trusted, and generate the direct port binding
for the address the hub uses. (b) require HTTPS for Edge hub URLs and TLS for
direct agents, except loopback.
