# Hub update experience, brand colours and local addresses

M30 (#529). The hub self-update keeps the loaded page usable through the restart, admins see
available hub updates in the navigation, gate screens look like the product, the logo keeps its
brand colours and hub addresses are local only.

## Decisions

- **Update view.** Starting a hub self-update opens a full-screen view: animated logo, fixed
  60-second estimate (bar to 90 %, then slower, never 100 % before the result), "taking longer
  than usual" after the estimate, run id and Close after 20 minutes. It stays in the already
  loaded page while the hub restarts.
- **Results.** Done needs the operation's healthy settlement, not only the new version
  answering: the helper still checks health for up to 120 seconds, and there is no automatic
  rollback once the new container runs. Results are done (reload after 10 seconds or on "Open
  Docklet"), failed (also when the new version runs but is unhealthy), unknown (the helper ended
  without a result) and sign-in required (no claim about the result).
- **Other tabs.** Only the tab that started the update, or a tab opened or reloaded during it,
  shows the view. Other tabs show the gate screen with automatic retry during the restart.
- **Navigation hint.** Confirmed administrators see a badge at Settings and at the Updates
  section when a hub update is available, with the version in a tooltip. A click focuses the hub
  update control. Nobody else triggers the version read.
- **Gate screens.** Loading, error and refusal states before the shell use the login page look.
  An unreachable hub (fetch failure, 502, 503, 504) retries automatically with a 5/10/20/30-second
  backoff. A full document reload while the hub is stopped shows the browser's error page,
  because the hub serves the document; there is no service worker.
- **Logo.** Mark and "DOCKLET" are violet and "NEXT" is cyan in both schemes, independent of the
  personal palette.
- **Local addresses.** The IP address field holds the host's IP (agents connect there) and the
  domain field an optional local name. Docklet does not model public exposure through a proxy.
  Discovery lists each address once with all of its origins (`settingsDomain`, `appUrl`,
  `settingsAddress`, `published`, `interface`, `request`): IP candidates are the same address when
  IP and port match (whatever the scheme), domain candidates when scheme, host (case-insensitive)
  and port match, with default ports filled in. IP-only validation applies when a value
  is written; stored legacy values are read unchanged and get an explicit "move to domain" hint.
- **Setup code.** Removed; see [first-run setup](first-run-setup.md).

Rejected alternatives: treating the new version answering as success (it can still fail health);
a service worker for offline documents (out of scope for a single-operator hub); learning the
duration at runtime (a fixed estimate is enough).

## Terminal decision for the update view

Each poll reads the operation (`/environments/0/system/upgrade/check`), the public version
(`/app-version`) and, when known, the run, each with its own timeout and without waiting for the
others. Only the operation and run of this update count.

| Evidence | Result |
|---|---|
| operation settled healthy | done |
| operation settled failed-health or failed-pre-effect | failed |
| operation settled unknown | unknown (wins over the failed run the hub records for it) |
| operation unreadable, run succeeded | done |
| operation unreadable, run failed with `update.outcome-unknown` | unknown |
| operation unreadable, run failed or cancelled otherwise | failed |
| run terminal, operation readable but not settled | still running for up to two more polls, then the rules above |
| new version answers, operation not settled | still running |
| every read fails (network, 502, 503, 504) | still running, hub offline |

A start whose response is lost never sends a second request: the view looks for a new operation
with the requested target, whether it is still running or already settled. A 401 while running
switches to sign-in required and keeps only public information; logout or another user signing
in clears the view.

## Break map

| # | Case | Guard | Test |
|---|---|---|---|
| 1 | hub offline | outage keeps running | gateway 502/HTML, fetch failure |
| 2 | target answers before health | done only on healthy settlement | health pending, then healthy, then failed |
| 3 | unknown outcome with failed run | operation outcome wins | settled unknown + failed run → unknown |
| 4 | session lost | sign-in view, public data only | real CSRF client and activity reader, 401 + public 200 |
| 5 | assets during outage | view and logo in the entry chunk | build graph, later asset requests failing |
| 6 | lost start response | read-only adoption of the new operation | response delayed past settlement, one POST |
| 7 | reload during update | recovery read outside Settings | real app on another route |
| 8 | logout | session cleared, no late reload | late poll after logout |
| 9 | setup without code | schema without `code` | legacy body 422 |
| 10 | candidate origins | one row per equivalent address | producer → DTO → mapper |
| 11 | IP grammar | write-time validation | IPv4, bracketed IPv6 with port |
| 12 | logo colours | fixed tokens | computed colours under another palette |
