# Identity and startup sign-in policy

Decision #68 implements the sign-in boundary of #58. One mode is selected by
`AUTH_MODE` at process start: `local` (the default), `proxy`, or `none`. Unknown or
combined values prevent hub startup. Agent and Edge listeners keep their own
transport authentication and do not use the hub sign-in policy. Hub mode changes
require operator configuration and a restart; the API exposes no mode mutation.

`local` authenticates the existing local administrator using a password, database
sessions, access tokens and browser cookies. The proxy identity header has no
meaning in this mode. The mutable legacy `authLocalEnabled` setting cannot select
another mode; API updates to it are rejected. Additional user identities and their
resource rights are outside this decision and belong to M15.

`none` maps every ordinary hub request to the global administrator. `proxy` maps
each accepted proxy identity to that same persisted administrator ID. Proxy names
are request-scoped activity attribution, not local accounts, role assignments or
password identities. The account is resolved by effective global administrative
permissions, so renaming its username does not change the mapping.

## Proxy trust boundary

`AUTH_PROXY_HEADER` defaults to `Remote-User`. `AUTH_TRUSTED_PROXIES` is a required,
comma-separated list of literal IPv4/IPv6 addresses or CIDRs in proxy mode. There
is no identity-trust default, including when the listener binds to loopback.
Invalid list members, all-address `/0` networks and invalid HTTP header names
prevent startup.

The hub parses the direct socket peer from `Request.RemoteAddr`. Framework client
IP extraction, `Forwarded` and `X-Forwarded-For` never establish identity. A peer
outside the list, a malformed peer address, a missing/empty identity or duplicate
identity headers returns 401. `TRUSTED_PROXIES` remains a separate forwarding
metadata setting and cannot authorize sign-in.
Identity values must be valid UTF-8 containing only printable characters;
controls are rejected before surrounding spaces are trimmed. IPv4-mapped socket
peers are matched against their unmapped IPv4 address.

The trusted proxy is responsible for authenticating its client, removing any
client-provided identity header and setting exactly one authoritative value. Each
additional hop must preserve that boundary at the trusted final proxy. Adding a
shared proxy network to the list authorizes every peer in that network to assert
administrator identity. The hub cannot distinguish compromised trusted peers
from legitimate proxy authentication. TLS termination and the connection to the
hub remain operator responsibilities.

## Exclusive access and session lifetime

Huma REST routes, Echo WebSocket/diagnostics streams and environment forwarding
share the startup policy. Proxy mode does not accept local password login,
refresh, local Bearer tokens, browser sessions or personal API keys as a second
sign-in path. In none mode local login/refresh/password endpoints are also
unavailable. Environment bootstrap keys, agent access tokens, Edge tunnel
credentials and webhook path tokens retain their existing machine contracts.
They do not become proxy user identities. Public API operations retain their
existing contracts, including health probes and the pairing handler that validates
its own environment bootstrap key; mode selection does not add a user sign-in
requirement to those operations.

Before serving requests, startup compares its mode with internal `authLastMode`.
A different or unrecorded mode atomically revokes existing sessions and records
the mode in the existing settings table. A failed write rolls the transaction back
and prevents readiness. Access/browser verification checks the mode before any
cache hit. Startup clears the verification cache; returning to local mode cannot
resurrect an older session. A same-mode restart preserves valid local sessions.
Restored database sessions are independently revoked by the backup subsystem.

## Browser request protection

Every ordinary state-changing hub request in every mode requires an exact allowed
Origin and a double-submit CSRF token. Allowed origins follow `APP_URL` and the
existing development-origin policy. Header presence alone never bypasses either
check; cookie-less proxy identity receives the same protection. In local mode,
validated Bearer tokens and personal/scoped API keys authenticate explicit
non-browser requests without Origin or CSRF tokens. Invalid or expired explicit
credentials return 401, allowing the CLI to refresh and retry. Cookie-only
authentication stays strict. Proxy and none modes never grant this local exception.
Startup normalizes the APP_URL scheme and host to lowercase and removes default
HTTP/HTTPS ports; nondefault ports remain part of the exact origin.

Local `POST /api/auth/login` and `POST /api/auth/refresh` also allow a missing
Origin only when no Cookie header is present. These endpoints authenticate the
explicit password/refresh token in the body and carry no ambient authority. A
browser Origin or any cookie requires the ordinary Origin and CSRF pair. The
shipped CLI uses these cookie-less flows; credentials still validate downstream.

A safe API GET/HEAD issues a random 32-byte token through `X-CSRF-Token` and a
host-only, HttpOnly, SameSite=Strict cookie. Trusted HTTPS context uses a Secure
`__Host-` cookie. Responses carrying bootstrap tokens use `Cache-Control: no-store`.
A writer sends the header back with the cookie. Missing/wrong Origin or a missing,
wrong or malformed cookie/header token pair returns 403. The public
`GET /api/auth/mode` is the documented bootstrap endpoint; its mode is read-only
and constrained to the three protocol values in OpenAPI and generated types.
The web HTTP adapter remembers this response header, bootstraps it before its
first write if necessary, and supplies it for both fetch writes and XHR uploads.

Only independently authenticated machine ingress keeps its separate contract:
agent-only listeners, validated environment-token requests, Edge tunnel tokens
and webhook path-token routes. Unvalidated Bearer/API-key or proxy headers do
not qualify. Local, direct environment and Edge-proxied WebSockets require the
same exact allowed Origin, including terminal upgrades. Request Host and other
localhost ports never expand that list. An absent Origin requires a validated
non-cookie credential; none/proxy browser streams cannot use an absent Origin.
Environment forwarding checks original client authority before injecting agent
tokens. This policy does not protect against same-origin script compromise or a
compromised authenticated proxy. It does not add per-user isolation.

## Operator recovery

`AUTH_RESET_ADMIN_PASSWORD` supplies a replacement password at startup. It must
satisfy the configured local password policy. After the normal database lock and
administrator initialization, startup atomically updates that administrator's
password and revokes its sessions. It clears cached token verifications, consumes
the in-memory value once per start and logs the reset without the value. Failure
prevents startup. Operators remove the environment value after use; leaving it in
the environment applies it again after the next restart.

Recovery is also possible by selecting none mode and restarting. There is no
password-reset CLI. All four startup variables support the existing `_FILE` and
`__FILE` loader variants; deployment and secret-file details belong to the
[hub package](../../deploy/hub/README.md). The active mode and token bootstrap are
backend contracts for M4-02; this decision adds no web UI screens.
