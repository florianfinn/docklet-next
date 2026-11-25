# Update version badges

M40 (#615, #616). Settings › Updates and the host cards show the installed product version and an
available update as badges. Both surfaces read the same self-update check. The change is web-only, with
no API, schema or adapter change.

## Decisions

- **One source.** Host cards use the existing self-update check, the same read as Settings › Updates,
  through one shared query. It is requested only for administrators (the check is an administrator
  route) and only for online hosts. A cold Hosts page load therefore costs one check per online host.
  For a remote host the hub forwards the check to the agent. Deriving the target in the browser from
  the host list was rejected: it would repeat the hub's eligibility rules and could contradict Settings.
  A new host-list field was also rejected because it needs an API change.
- **Ownership.** For an agent, the cached check belongs to the host's registration revision, Edge
  session and instance, connect time and reported version. A replacement registration, a reconnect or
  a new reported version reads afresh, and an answer for a superseded identity is never shown. The hub
  keeps its existing key, which the hub update session invalidates.
- **Authority.** Only a successful check decides the badge tone. When it does, its own version is the
  installed version: "current" in the success tone, or the installed version next to a highlighted
  target. Without a successful check (not an administrator, host not online, pending, failed), the
  reported version stays neutral and has an accessible reason.
- **Settings panel.**
  - Each host shows one header line with its badges.
  - Update stays the primary action. When no update is possible it stays visible and disabled.
  - Manual commands sit in a disclosure that is open by default only when the automatic update is not
    possible.
  - Version hints drop the repeated hub version and repeated references, and shorten digests with a
    copy action for the full value.
  - On the hub's own host the agent row is left out, because no agent image runs there to compare.
  - Orphaned restore points render only with entries or a read error.
- **Card link.** The target badge links to the host's panel (`#hub-update` for the hub,
  `#agent-update-<id>` for agents). That panel alone scrolls into view and focuses its Update button,
  or its heading when the button is disabled.
- **Simple users.** The Hosts page stays administrator-only ([simple-users.md](simple-users.md) §4),
  so simple users see no host cards and no update check runs for them. Independently of the route,
  the check, the target badge and the link require the `selfUpdate` capability.

## Proofs

Unit and component tests cover:
- the badge model over all check states;
- host-identity replacement with a late answer;
- request and render gating for hosts that are not online and for viewers without the capability;
- anchor focus, the disclosure rule and the hub-host hint row;
- Direct/Edge parity and that shared query data is never modified.

Practical acceptance in the sandbox follows the milestone matrix on closing issue #617.
