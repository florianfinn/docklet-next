# Container shell

An interactive shell for a running container in container detail, on hub-local,
Direct and Edge hosts. The [core adoption](core-adoption.md) scope permits the
shell for Unraid containers, and restricted users in M15 keep it. Shared
contract and migration items are in
[container and host operations](container-host-operations.md).

## Decisions

- The shell choice is `auto` (bash if present, otherwise sh), `sh` or `bash`;
  `auto` is the default. Arbitrary paths are refused.
- There are no session limits and no idle timeout. Docklet runs at small scale
  with one operator and a few users; limits would add rules without a matching
  risk.
- The activity log records that a shell started and ended, with actor,
  environment, container, times and end reason. It never records content.
- The shell is allowed for Compose, standalone, Unraid and unknown containers.

## Design

1. **Hub acceptance hook**, one function called before any upgrade: for
   environment 0 inside the local handler, for Direct and Edge in the
   environment middleware before the WebSocket or tunnel proxy, for the terminal
   route only. It validates `shell` ∈ {auto, sh, bash} and optional `cols` and
   `rows` (1–1000); anything else is HTTP 400 `container.shell-invalid` without
   an upgrade. It records the hub user as actor (never the agent's manager
   identity), opens the activity entry and returns a release handle that runs
   exactly once on any end: failed upgrade, failed dial, close, EOF or
   revocation.
2. **Command**: `auto` runs
   `/bin/sh -c 'if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi'`;
   `sh` and `bash` run `exec <name>` under `/bin/sh -c`. A missing `/bin/sh`
   ends the session with `container.shell-unavailable`.
3. **Frame protocol v1**: binary frames carry stdin and stdout; text frames carry
   JSON control. The client sends `{"type":"resize","cols":N,"rows":M}` with
   each dimension in 1–1000, or `{"type":"close"}` to request process cleanup
   while keeping the exit channel open. Unknown members or other controls end
   the session with `container.shell-protocol`. The server
   writes a final `{"type":"exit","code":N|null,"reason":"<code>"}` before
   initiating close while the connection remains writable, including failures
   after the upgrade (container not running, create or attach failure, revocation). The browser treats the exit frame as
   authoritative; WebSocket close codes are 1000 or 1008 only. An unknown end
   has `code:null` and an empty reason; `container.shell-unavailable` means only
   a missing `/bin/sh`. A plain HTTP GET may recover an upgrade refusal without
   creating another shell activity.
4. **Close-reason transport**: the Edge tunnel close message carries the
   upstream close code and reason, and the hub bridge forwards them. Direct
   proxying preserves binary and text frames. Close metadata uses the existing
   tunnel close error field; older plain error strings remain readable.
5. **Process end**: Ctrl-D, `exit`, then `ExecInspect` after 2 s. A process that
   still runs has its hijacked connection closed and the activity records
   `container.shell-orphaned` (an accepted limit: such a process may survive).
   An inspect error cannot prove an orphan. When the browser closes first, the
   Direct or Edge hub bridge sends `{"type":"close"}` to the agent and keeps
   that channel open for its final exit, for at most five seconds after browser
   close. The final agent reason reaches activity persistence even though the
   browser is gone. A dropped agent connection or expired bound ends the existing
   activity without a shell-specific code; it never invents shell-unavailable.
   Cleanup joins the input/output pumps and releases the exact accepted handle.
6. **Edge `ws_start`**: the agent validates it with `ValidateEdgeCommand` like a
   stream open; absent and mismatched commands are refused.
7. **Web**: `@xterm/xterm` and `@xterm/addon-fit` (MIT, recorded licences, bundle
   size budget). A Terminal tab in container detail maps HTTP problems and exit
   reasons to EN/DE texts, offers reconnect and is disabled with a reason when
   the container is not running or the host is unavailable; standby is available.
   The `openContainerShell` descriptor requires `containers:exec` without an
   origin restriction. The terminal loads lazily, with a 160 KiB gzip budget for
   its JavaScript and CSS assets. The demo adapter
   plays a scripted echo session.
8. **Revocation**: a session registry closes sessions by user; M15 uses it when
   rights change. The registry lives in `internal/containershell`; each acceptance
   owns a distinct handle and activity. Application shutdown, session revocation
   and remote credential changes cancel that handle, and repeated or delayed
   release cannot touch a replacement session.

Codes: `container.shell-invalid`, `container.shell-protocol`,
`container.shell-unavailable`, `container.shell-orphaned`,
`container.shell-not-running`.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
| --- | --- | --- | --- | --- |
| SH1 | invalid shell or size query | 400 before upgrade on all transports | hub acceptance hook | request through the hub URL for each transport → 400, no upgrade (mutation: validate after upgrade or at the agent) |
| SH2 | legacy `ws_start` | validated like a stream open | agent client requests | real tunnel: absent and mismatched command to a forbidden path → no local dial (mutation: skip) |
| SH3 | resize | bounds, `ExecResize` | exec input loop | real WebSocket → parser → Docker HTTP test server; out of range → `shell-protocol` (mutation: ignore resize) |
| SH4 | frame types | preserved | Edge, both directions | real Edge test with binary stdin, text resize and text exit (mutation: coerce) |
| SH5 | orphan | inspect and record | close path | runtime double that ignores exit → orphaned recorded, cleanup joined (mutation: omit inspect) |
| SH7 | revocation | registry closes by user | hub registry | only the matching user's sessions close (mutation: omit closer) |
| SH8 | codes | shell codes above | types, OpenAPI, v11 fixture, EN/DE | catalogue and fixture tests |
| SH9 | EOF | exit frame, then close | attach read loop, remote too | real Direct and Edge close delivery (mutation: suppress the exit frame) |
| SH10 | early failure semantics | pre-upgrade HTTP versus post-upgrade exit frame, preserved through Direct and Edge | hook → bridge → agent → terminal | create failure, invalid control, idle session over real transports (mutation: invent a close code) |
| SH11 | single release, handle identity | release once per handle; a stale callback never touches a replacement session | registry and cleanup over real Direct and Edge paths | concurrent EOF, cancel and logout; held output, then a host token change or removal and re-registration, an immediate replacement session and delayed old cleanup → the replacement stays live, actors are correct, one close per session, counters correct (mutation: release twice, or release by user and container instead of handle) |

D4 adds the shell capability descriptor as an authorized foundation change. D5
adds the explicit close control and bounded exit drain to the unreleased v11
contract. The generated schema, v11 fixture and fingerprint include the control;
contract version and minimum remain 11. No database migration changes.
