<!--
Source: docklet-next/issues/331
Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
Retrieved: 2026-11-03
License: Apache-2.0
Deviations: Original service widget architecture decision.
-->

# Service widgets and app links

Decision for #331 (architecture), #332, #333 and #334 (M17, maintainer kickoff
2026-11-03). Accepted before: the hub fetches, credentials are encrypted, every signed-in
user sees the widgets and only administrators configure them (core adoption R8 and R10).

## 1. Model

- One widget per kind: Plex, Overseerr (the Jellyseerr API is the same) and Minecraft
  Java. Each has an enable switch, an address, an optional credential, an optional link
  or stack and, for https, an optional trusted certificate.
- Plex and Overseerr addresses are http or https base URLs without user information or
  query; Minecraft uses `host:port` (default 25565). Private and loopback addresses are
  allowed, because these services run on the local network.
- The Plex token and the Overseerr API key are encrypted with the hub's credential key
  and never returned: the API reports only whether a credential exists, and an update
  keeps, replaces or clears it. A missing or changed key yields
  `widget.credential-unavailable`, never plain text.
- https always verifies certificates; there is no switch that turns verification off. If
  verification fails, the test shows the SHA-256 fingerprint of the server certificate
  and the administrator can trust exactly that certificate for this widget. The pin is
  bound to the configured host and port; a replaced certificate fails again until it is
  trusted. A pinned certificate is accepted on its fingerprint alone, without a name
  check, so self-signed certificates on LAN addresses work; without a pin the usual
  chain and host name verification applies.

## 2. Fetching

- On demand: a widget read returns the cached result while it is younger than the kind's
  interval (Plex 15 s, Minecraft 30 s, Overseerr 60 s); otherwise one fetch serves all
  waiting readers. Nothing is polled while nobody looks.
- Each fetch has a 5 second total budget and a 1 MiB response limit (64 KiB for the
  Minecraft status). Redirects stay on the same scheme, host and port, at most three.
- After a failure the next attempt waits 30 s, 60 s, 2 minutes, then 5 minutes. In between
  the read returns the last good data marked stale with its age and the error, or the error
  alone.
- Fetches and caches are keyed by kind, configuration revision and a generation that a
  saved configuration or an Overseerr action advances; a result for an older key is never
  returned or cached. Results are immutable and readers get copies. The test action uses
  its own transient fetch and leaves the live cache untouched.
- Error codes: `widget.not-configured`, `widget.disabled`, `widget.unreachable`,
  `widget.timeout`, `widget.auth-failed`, `widget.bad-response`,
  `widget.redirect-refused`, `widget.tls-failed`, `widget.credential-unavailable`.

## 3. Drivers

- **Plex:** current sessions with user, title, subtitle, device, progress, quality and
  direct play or transcode (at most 20); totals of streams and transcodes.
- **Overseerr:** counts of pending, approved and available requests and the five newest
  pending requests with title, media type, requester and age; titles are cached per
  revision, generation, media type and id. The requester is the display name, user name
  or Plex name; the email address never leaves the hub, because widget data reaches every
  signed-in user. Administrators can approve or decline a request; the
  action is recorded as an activity.
- **Minecraft:** the Java server list ping: players online and maximum, up to twelve player
  names, message of the day without formatting codes, and version. No TPS, no session
  times and no RCON password.
- Strings are capped at 200 characters and counts at the limits above; a response
  without its required fields is `widget.bad-response`.

## 4. Routes, access and interface

| Route | Access |
| --- | --- |
| `GET /api/service-widgets`, `GET /api/service-widgets/{kind}` | every signed-in user |
| `GET`/`PUT /api/service-widgets/{kind}/config`, `POST /api/service-widgets/{kind}/test` | administrators |
| `POST /api/service-widgets/overseerr/requests/{id}/approve` and `/decline` | administrators |

The module runs only in the hub, not in agent or Edge mode. Settings › Service widgets
configures each kind and tests a draft before saving. Dashboard cards replace the former
development-only cards and show loading, not configured, error, stale and data states.
Simple users get a reduced dashboard with the configured service widgets and the apps
widget of their assigned stacks, without infrastructure data or Overseerr actions.
Their Dashboard navigation entry and landing route lead there; the administrator
dashboard and its host, update, security, schedule and event reads never mount for them.

The apps widget lists stacks whose Compose metadata names an http or https address
(`x-arcane: urls`); the first such address opens in a new tab.

## 5. Break map

| ID | Row | Test |
| --- | --- | --- |
| SW1 | external data per driver | fake servers with valid, oversized and malformed responses |
| SW2 | egress limits | real test servers: slow, endless body, foreign redirect, loop |
| SW3 | failure isolation | a slow widget does not delay another |
| SW4 | backoff and stale data | injected clock |
| SW5 | late result after a save or an action | revision and generation keys, equal movie and series ids |
| SW6 | shared results | a reader mutating its copy changes nothing else |
| SW7 | credentials | encrypted row, write-only API, missing or changed key |
| SW8 | test action | no write and no change to the live cache |
| SW9 | access and simple-user surface | data for users, configuration and actions for administrators, real user session |
| SW10 | hub only | absent in agent and Edge mode |
| SW11 | error codes | contract and rendering |
| SW12 | migration | upgrade with the new table |
| SW13 | Overseerr actions | upstream request, activity, refreshed list |
| SW14 | apps widget | address mapping |
| SW15 | certificate pin | self-signed, trusted, replaced and foreign-host certificates |

## 6. Limits

- One server per kind; legacy Minecraft servers before 1.7 and Bedrock are not supported.
- Acceptance uses fake Plex and Overseerr servers built from the documented API and a real
  Minecraft server; other server versions may differ.
