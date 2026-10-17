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
   (`pkg/remenv/client.go`, `ApplyAgentTokenHeaders`). Arcane also passes the
   calling user's hub credentials across this boundary (T11).
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
`internal/environment/service.go`); deleting a host cascades its key. Both reject
new authentication with the old key: hub API calls, Edge registrations and each
Edge poll request resolve the token again. Neither revokes an established
session (T4).

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

Rotation does not end an established WebSocket or gRPC tunnel. The session
authenticates once at registration; later frames and heartbeats are not checked
against the current token (`server.go`, message loop and `handleHeartbeat`).
Forwarding selects the tunnel by environment ID and injects the current token
(`proxyActiveEdgeTunnelInternal` and `setProxyContextHeadersInternal` in
`internal/middleware/environment.go`), so an impersonator learns the replacement
token from the next proxied request. `ApplyEnvironmentRuntimeState` even reports
the host online over its persisted `pending` state while that tunnel exists.
Deleting the host rejects new registrations and stops ordinary forwarding once
the row is gone, but leaves the tunnel open.

Residual risk: equal to T3 plus disclosure of what the hub sends to that host,
and incident recovery by rotation fails against a connected impersonator.
Verdict: **needs change** (review decision B2): rotation and removal revoke
established sessions. The Hosts page shows connection events and the connected
agent instance; D4 covers mTLS.

### T5. Compromised host reaches beyond itself (H)

H owns its host already. Beyond it, H can read the token (T3, T4 for this host
only), receive every enabled registry credential of the hub, because the sync
sends the full set to each agent (`GetEnabledRegistryCredentials`,
`SyncRegistriesToEnvironment` in `internal/environment/service.go`), report false
inventory, and emit misleading events. It cannot use its token for another host:
hub grants come from `EnvironmentPermissionSet` (`pkg/authz/permission_set.go`)
and tunnel registration resolves one environment ID per token.

H also receives the hub credentials of users whose requests reach it; that
crossing is T11.

Residual risk: registry credentials leak to every host, and that host's own view
is untrustworthy. With one operator who uses the same registries everywhere this
matches the operator's own trust in each host. Verdict: **accepted**; the
[agent surface](agent-surface.md) row for `registry` already records it.

### T6. Malicious agent responses reach the browser on the hub origin (H, T)

(baseline before #81) The hub copies the agent's response headers to the browser, removing only
hop-by-hop headers: `ProxyHTTPRequest` in `pkg/libarcane/edge/proxy.go` for Edge
and `writeProxyResponse` with `CopyResponseHeaders` in
`internal/middleware/environment.go` for direct hosts. No global
`Content-Security-Policy` or `X-Content-Type-Options` header is set for proxied
responses. A malicious agent can therefore return `Set-Cookie` for the hub origin
or HTML/script with an active content type on an environment path. If an admin
opens such a path, for example a download link, script runs with the admin's
session and can command every host. `X-Content-Type-Options: nosniff` alone would
not help: it stops type guessing, not script in a correctly labelled `text/html`
or `image/svg+xml` body. Response size is a separate threat (T12).

Residual risk: one compromised host or token escalates to the hub and all hosts.
Verdict: **needs change** (D2 and review decision B1): filter proxied response
headers and confine active content with a hub-owned sandbox policy. After both,
an agent can still show static, script-free content on an environment path; it
cannot run script, submit forms or read the hub origin's cookies.

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

(baseline before #81) Arcane enforces no agent version. The hub publishes `agentContractVersion`,
`agentContractFingerprint` and `minimumAgentContractVersion` in its version info
(`internal/version/service.go`, `types/contract/agent.go`: both versions are 2),
and an agent returns the same structure on `/api/app-version`. No code compares
them; `GetEnvironmentVersion` in `internal/environment/handler.go` only reads
and returns the value. The Edge register message carries token, instance ID,
capabilities and resume session, but no version (`proto/tunnel/v1/tunnel.proto`,
`RegisterRequest`); capabilities are feature flags such as `proto-parity-v1` and
`chunked-request` (`pkg/libarcane/edge/command_client.go`). Per-message size
limits are enforced on both sides (T12), and stale sessions are reaped every
minute (`StartCleanupLoop`).

Residual risk: an old agent may lack checks that the hub relies on, or silently
misread changed payloads. #81 requires enforcement. Verdict: **needs change**
(D1).

### T9. Resource exhaustion on tunnel endpoints (N)

`/api/tunnel/connect` is rate limited per IP (60 per minute, burst 30) and per
token (10, burst 3); `/api/tunnel/mtls/enroll` per IP and per token (10, burst 3);
`/api/tunnel/poll` has no rate limit (`registerEdgeTunnelRoutes` in
`internal/bootstrap/edge_bootstrap.go`). Valid tokens are cached; misses query
SQLite (`ResolveEdgeEnvironmentByToken`). Each tunnel message is bounded
(T12), and stream delivery waits at most `streamDeliveryTimeout` (5 seconds).

Residual risk: an unauthenticated flood on the poll endpoint costs one indexed
lookup per request. At this scale a reverse proxy or firewall in front of the hub
is the expected control. Verdict: **accepted**.

### T10. Token exposure during onboarding (T)

Arcane's snippets embed the token as `-e AGENT_TOKEN=<token>` in a `docker run`
command and inline in compose (`snippets/service.go`). That places it in shell
history, process listings and copied compose files. Arcane also accepts the token
from a file (`AGENT_TOKEN` with `options:"file"` in `internal/config/config.go`).

The frozen reference does not confine the token to a protected `.env` either.
`agentScript` embeds the `.env` content, token included, in `install.sh`
(`web/src/domain/agent-install.ts`). The download packs `install.sh` with mode
755 and `.env` with the tar default 644 (`InstallArtifacts.tsx`, `createTar` in
`web/src/shared/lib/tar.ts`). The copyable script command writes that script to
the fixed path `/tmp/install-docklet-agent.sh` without restrictive permissions
or cleanup. The script and the compose command create `.env` before
`chmod 600`, without a private umask. Each copy is readable by other local users
who have no Docker access.

Residual risk: with the generator rules of review decision B4 in
[host onboarding](host-onboarding.md), the token stays visible in
`docker inspect` to anyone with Docker access on that host, who has root there
already. The copyable commands write `.env` through a heredoc, so a pasted token
can stay in interactive shell history; the downloadable package avoids that. The
downloaded archive holds the token until the operator deletes it. Verdict:
**accepted** with those generator rules, which are new work in #84.

### T11. Hub user credentials forwarded to agents (H, T)

(baseline before #81) When the hub forwards a user request, it also passes that user's hub
credentials. Direct HTTP and WebSocket forwarding copy the caller's
`Authorization` header or, without one, turn the hub session cookie into
`Authorization: Bearer <session token>` (`SetAuthHeader` and
`BuildWebSocketHeaders` in `pkg/libarcane/edge/remenv.go`, called before
`SetAgentToken` in `internal/middleware/environment.go`).
`ApplyAgentTokenHeaders` in `pkg/remenv/client.go` deliberately leaves
`Authorization` in place; it overwrites only a caller's `X-API-Key`. Edge HTTP
and WebSocket forwarding drop `Cookie` but pass an explicit caller
`Authorization` header into the tunnel (`ProxyHTTPRequest` in
`pkg/libarcane/edge/proxy.go`, `buildWebSocketHeaders` in `ws_proxy.go`). The
Edge agent strips it only when it replays a WebSocket locally
(`buildLocalWebSocketHeadersInternal` in `client.go`), after the agent process
has received it. Existing tests assert this forwarding
(`TestSetAuthHeader_ForwardsAPIKeyAndAuthorization`,
`TestSetAuthHeader_UsesCookieTokenWhenNoAuthorization`,
`TestBuildWebSocketHeaders_UsesAuthorizationHeaderAndAddsAgentToken`,
`TestBuildWebSocketHeaders_UsesCookieTokenAsBearer` in `remenv_test.go`).

The agent does not need these values: `agentAuth` grants its superuser from the
agent token alone (`agentSudoInternal` in `internal/auth/middleware_echo.go`). A
compromised direct host therefore collects the session token of every user whose
browser reads it, during ordinary inventory requests and without any user
interaction, and can replay it as the hub session cookie. An explicit access
token can be replayed as a Bearer credential; on Edge this applies to explicit
`Authorization` headers. A token thief who wins an Edge session (T4) collects the
same. Raw `Cookie` stripping does not prevent any of this.

Residual risk: compromise of one host becomes hub administrator access and so
control of all hosts. Verdict: **needs change** (review decision B3): hub user
credentials never leave the hub boundary; agents receive only the agent token.

### T12. Authenticated agent exhausts hub memory (H, T)

The tunnel limits apply per message: 16 MiB per gRPC message
(`maxGRPCTunnelMessageSize` with `MaxRecvMsgSize` in `server.go`) and 32 MiB per
WebSocket message (`maxWebSocketTunnelMessageSize` with `SetReadLimit` in
`tunnel.go`). They do not bound a response body. For Edge HTTP proxying the hub
appends every response, stream and data chunk to one in-memory buffer without an
aggregate byte cap (`grpcResponseState` in `pkg/libarcane/edge/proxy.go`) until
the response ends or the proxy deadline passes, and then writes it as one blob
(`ProxyHTTPRequest`). An Edge agent, or a token thief holding its session (T4),
can therefore send many valid small messages and grow hub memory until that
deadline. Direct responses are copied to the browser as they arrive
(`CopyBodyWithFlush` in `remenv.go`); they have no tunnel message limit and are
not collected in hub memory.

Residual risk: an authenticated agent can slow or crash the single hub process by
memory exhaustion; a restart recovers it. This is separate from the
unauthenticated poll flood in T9. At the product scale the agent's holder already
controls a host the operator chose to trust. Verdict: **accepted**; no aggregate
body cap is added in M5 (review decision S2).

## Summary

| Threat | Position | Verdict |
| --- | --- | --- |
| T1 Direct eavesdropping | N | accepted |
| T2 Edge eavesdropping or hub impersonation | N | accepted |
| T3 Stolen token controls its host | T | accepted |
| T4 Stolen token impersonates Edge agent | T | needs change |
| T5 Compromised host reaches beyond itself | H | accepted |
| T6 Agent responses on hub origin | H, T | needs change |
| T7 Compromised hub or backup | M | accepted |
| T8 Incompatible or downgraded agent | N, T, H | needs change |
| T9 Tunnel endpoint exhaustion | N | accepted |
| T10 Token exposure during onboarding | T | accepted |
| T11 Hub user credentials forwarded to agents | H, T | needs change |
| T12 Authenticated agent exhausts hub memory | H, T | accepted |

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

## Review decisions

The maintainer decided the following after the first independent review of
this record on 2026-10-17. Items that name an issue are new work there.

- **B1. Sandbox policy on proxied agent responses (T6), #81.** In addition to the
  D2 header allowlist, the hub sets `Content-Security-Policy: sandbox;
  default-src 'none'` on every proxied agent HTTP response, whatever its content
  type, and replaces any policy the agent sends. The policy confines active content when a response is opened as a document
  and also applies to worker globals. The intended JSON fetch reads and file
  downloads do not execute the response as active content and need no exception. Direct and Edge
  forwarding use one shared response filter for allowlist, `nosniff` and policy;
  where a transport copies agent headers onto a WebSocket handshake response,
  the same filter applies. Verification on both transports: an agent response
  with `text/html` and with `image/svg+xml`, each with inline script and no
  `Content-Disposition`, reaches the browser with the policy, `nosniff`, no
  `Set-Cookie` and only allowlisted headers.
- **B2. Rotation and removal revoke established sessions (T3, T4), #81.** Two
  effects are kept apart. Rejecting new authentication with the old key exists
  in Arcane. Revoking established sessions is new: rotating a host's token or
  removing the host closes its Edge WebSocket or gRPC tunnel, drops its poll
  session state and ends open proxied streams such as shells and logs, for
  direct and Edge hosts. Each tunnel records the API key ID it registered with;
  forwarding injects the agent token only into a tunnel whose key ID is current
  and closes any other, so a new token is never delivered through a session
  authenticated with the old token. Runtime state reports a host online only for
  a tunnel with the current key. Verification: after rotation and after removal,
  an Edge session registered with the old token is closed, receives no request
  carrying the new token, and a new registration with the old token is rejected.
- **B3. Hub user credentials stay at the hub (T11), #81.** Before forwarding,
  direct and Edge transports, HTTP and WebSocket alike, remove the caller's
  `Authorization`, `Cookie` and `X-API-Key` headers and the configured
  `AUTH_PROXY_HEADER` ([identity](identity.md)), and convert no cookie into a
  Bearer header. They then set only the agent token headers. The four tests
  named in T11 confirm the current forwarding and change in #81 to assert its
  absence.
- **B4. Installer token handling (T10), #84.** The token lives only in a
  mode-600 `.env` created under a private umask; details are in
  [host onboarding](host-onboarding.md#review-decisions).
- **S2. Per-message limits (T12).** Frame limits are described per message. The
  accumulated Edge response body is an accepted residual at single-operator
  scale; no new byte budget or transport change follows.
- **S3. Compatibility before forwarding and sync (T8, D1), #81.** The hub
  establishes an agent's compatibility before any ordinary forwarding and before
  any configuration or registry sync. This includes the sync triggered by
  pairing, creation and update (`triggerEnvironmentResourceSyncInternal` in
  `internal/environment/handler.go`) and the health job, which today pushes
  registry credentials right after a successful connection test
  (`runHealthCheckInternal` in `internal/environment/service.go`). Until then
  only health, version and agent self-upgrade are forwarded, as D1 states. D1's
  version read after registration and the unchanged tunnel protocol stay.
