# Proxy UI adoption

Decisions for #549, with #541 and #503. The production proxy module
(`web/src/features/proxy`) adopts the presentation of the reviewed click
prototype ([proxy UI concept](proxy-ui-concept.md), `features/design-lab/proxy`)
on the real model of the [proxy module](proxy-module.md). It adds select lists,
suggestions and tooltips where Docklet knows the values (maintainer decision
D-3).

## Decision

**Presentation only.** The prototype edits a browser-local route model. The
module edits `Route`/`Change` DTOs produced by the agent. Layout, section
structure, row design and tooltips carry over. The lab model, text round trips,
lab bridge and store do not.

**Copy, never import.** Adopted pieces are copied into `features/proxy` or the
existing `web/src/shared` components. Each copy gets a provenance header that
names the lab source path and the revision. The production bundle guard and
the source guards in `review-fixes.test.tsx` stay as they are; for example,
`[overflow-wrap:anywhere]` replaces `break-words`.

**Page.** `PageHeader` replaces the module's own heading and wrapper. An
instance bar card with icon tile, instance picker and host badge follows the
lab's `ProxyInstances`. Line tabs follow `ProxyTabs`: Routes and Settings. The
lab's Certificates and Route map tabs are left out (no API; AUTO-5).

**Route list.**
- **Layout.** Summary tiles: routes, with TLS, label routes, read-only. Search
  across domain, service and stack, a source filter, the route count and
  "Add route".
- **Rows.** Rows use the shared overview table, the same component family as
  the stack and container lists, with icon tile, domain, destination with link
  icon, TLS lock with resolver, a source badge (File / Docker labels) and a
  status badge (Editable / Read-only / Infrastructure) in `ToneBadge` colours.
- **Narrow screens.** Card rows below `md`.
- **Certificate details.** The served-certificate check stays per row. Its
  expanded details span the full row width under the grid (#541).

**Route editor.**
- **Modes.** The dialog keeps Easy and Advanced as a segmented toggle.
- **Sections.** Easy shows the prototype's titled, colour-toned sections:
  1. Domain and matching
  2. Destination
  3. TLS
  4. Handling (middlewares, priority)

  At desktop width they are stacked. On narrow screens they become step cards
  with Previous and Next (AUTO-6, as in the lab's Manager design).
- **Tooltips.** Every field has a help text on hover and on keyboard focus,
  using the shared `Tooltip`.
- **Destination.** Choice cards for "Container / stack service" and "External
  service". The container choice offers the running containers on the proxy's
  host (`hostId` equals the instance's environment) from the existing container
  inventory.
  - A container that shares no network with the proxy container stays
    selectable and shows a warning (AUTO-7). The proxy container is matched
    through the composite inventory ID of the instance's raw container ID.
    Unknown network data shows no warning and no claim either way.
  - Picking a container fills the host with the unique container name, never a
    generic service alias. Only TCP container ports are offered. One port is
    filled directly. Several ports open a choice, and no port leaves the port
    empty. The scheme stays editable and defaults to `http`.
  - Fills happen only on a deliberate pick. An inventory refresh never changes
    the draft. If the picked container disappears, the draft keeps its values
    and shows a hint.
  - Host and port changes go through the existing `backendParts`/`backendURL`
    split, so `rest` and `colon` are kept. Secondary upstreams stay untouched.
  - Shared and opaque services show no suggestions, as today.
  - The external choice keeps scheme, host and port fields.
- **Middlewares.** Toggle chips over the middlewares the proxy observation
  lists, each with help text by reference kind (local, plugin, internal,
  external). Help text per middleware type needs the type from the agent and is
  a follow-up (AUTO-8). Free entry stays possible for names not observed yet.
- **TLS (AUTO-9, revised after design review).** The current `Change` cannot
  remove a router's TLS block, because an empty resolver is a no-op in
  `traefik/prepare.go`. The section therefore offers only what the contract
  expresses, and the wire stays unchanged:
  - **New route:** "No route certificate" (no resolver, nothing TLS-related
    sent) or "Certificate from resolver". The resolver is a select over the
    inventory resolvers with their challenge summary. "No route certificate"
    only promises that the route has no TLS setting of its own. When a selected
    entry point has static TLS, the choice says that this entry point still
    serves TLS. When no entry point is selected, it says that the default entry
    points apply and may serve TLS. The existing editor text for these cases
    stays.
  - **Existing route:** "Keep current", which preserves only the resolver
    (entry points stay editable and are sent when changed, AUTO-16), or
    "Certificate from resolver" (change the resolver).
    Removing TLS from an existing route stays in Advanced and is tracked as a
    follow-up. That follow-up would need a clear-TLS field and contract 21.
  - Entry points stay an explicit checkbox list inside the TLS section. A TLS
    choice never adds or removes entry points automatically. The existing text
    for implicit default entry points and router TLS versus entry-point TLS
    stays.
  - The lab's wildcard scope cards are left out, because they were simulated
    there.
- **Domain.** Suggestions come from domains already used by routes of the
  instance, which are parsed from simple `Host()` rules. A domain that is
  already in use shows a warning before save.
- **Unchanged behaviour.** Enable/disable and the redirect switch have no DTO
  field and are left out (AUTO-10).

**Label routes.** The lab's readable summary (match, target, TLS, middlewares)
uses the `Route` fields, and "Open in stack editor" uses the existing link.
There is no raw-label view, because the DTO carries no label text.

**Settings and setup.** Cards with titles, `Detail` rows and badges replace
the plain bordered boxes. Buttons are not full width. The production logic
stays unchanged.

**#503 (AUTO-11, revised).** `Instance.source.projectPath` is a mount path
relative to the projects root, and single-file sources have none, so path
equality cannot identify the owner.
- The owner is instead the stack membership of the proxy container itself,
  proven from its own Compose labels. The instance's host and raw container ID
  give the inventory row (`inventoryId(environmentId, containerId)`).
- Its `com.docker.compose.project.working_dir` label must match exactly one
  stack `path` on the same host. Both use host coordinates, because the
  projects directory is mounted at the same path.
- On a unique match the link goes to `/stacks/<id>?tab=editor`. Otherwise the
  hint stays plain text.
- The display-owner selection in `project-ownership.ts` (`stackId`) is never
  the authority for this link.
- No DTO change is needed.

**Texts.** New keys live in `proxyContract.ui` (en, de). German texts follow
the lab's German strings where they fit the production vocabulary.

## Contract

No API or agent change. All suggestions use existing reads: `useContainers()`,
`ReadResult.middlewares`, `Inventory.resolvers` with `resolverSummaries`,
`Inventory.entryPoints`, `ReadResult.routes` and `useStacks()`. The write path
is unchanged.

## Write-path invariants (must stay true)

- Only changed leaves are sent. Untouched backends keep their exact bytes.
- `expected` and `graphHashes` come from the read held while the dialog is
  open.
- Caddy Easy produces site text, and an edited Advanced text is authoritative.
- The acknowledgement gating (unresolved references, running divergence,
  restart), the locks (pending, recovery, uncertain outcome) and the refusal
  handling are unchanged.
- The shared-service and opaque-service rules are unchanged, and so is the
  `PRESERVE` sentinel.
- The existing write-path suites stay green: `ProxyPage`, `review-fixes`,
  `editor-lifecycle`, `closing-review-*`, `Operations`, `CertificateCheck`,
  `ResolverDialog`, `setup-*`, the decoder and outcome tests, and the
  production boundary. Selector changes in those tests are deliberate and
  named in the commit.

## Break map

| # | Row kind | Case | Decision | Entry boundary | Test | Red mutation |
|---|---|---|---|---|---|---|
| 1 | write path | picking a container suggestion on an existing route | only `upstreams` sent if host/port changed; other leaves untouched | editor `save()` | exact `Change` assertion | send all leaves |
| 2 | write path | TLS: new route "No route certificate" / "Resolver" with plain, static-TLS, mixed and unknown default entry points; existing route "Keep current" / "Resolver" | exact `Change` per choice and the real Traefik patcher output: no route `tls` key; inherited TLS stated in the UI for static-TLS, mixed and default cases; Keep leaves `entryPoints` and the `tls` subtree byte-identical | editor TLS mapping; `traefik/prepare.go` test fixture | Change test, UI text test, Go patcher test | present absent router TLS as "no TLS" despite listener TLS / send `resolver:""` for Keep |
| 3 | write path | middleware chip toggled on a name with a comma | names list keeps the exact name | chips → `middlewares` | test | split on comma |
| 4 | external data | container stopped, on another host, with UDP-only ports, several ports, no port; refresh during an open draft; picked container removed | host/state filter; TCP only; choice for several; no fill on refresh; hint on removal | suggestion filter and pick handler | tests per case | drop the host filter / fill on refresh |
| 5 | external data | container not on a proxy network | offered with warning | suggestion filter | test | hide it |
| 6 | external data | rule not parseable for domain suggestions | ignored for suggestions, shown as-is in the list | domain parser | test with `HostRegexp`, compound rules | throw on parse |
| 7 | bundle | adopted code in production graph | no design-lab import, no lab strings | production boundary guard | existing guard + `check-production` | import from design-lab |
| 8 | layout | expanded certificate details at 1440 px | full-width row below the grid | `RouteList` | DOM structure test + screenshot cell | render inside the auto column |
| 9 | link | #503: proxy container whose `working_dir` label matches one stack path; same-host duplicate-name stacks in different directories; no match; same raw ID on two hosts; single-file Caddy source | link only on a unique same-host label match, `?tab=editor` | hint component | producer-shaped fixtures | reuse the display-owner `stackId` / omit the host |
| 10 | step editor | narrow-screen steps hide invalid fields | Save stays reachable on the last step and validates all sections; the first invalid field's step opens and the field gets focus and an announcement; Enter on a non-final step goes to Next; the draft survives step and mode changes; acknowledgements stay visible before Save; a definite refusal reopens the editor with the draft | editor | tests | validate only the current step |
| 11 | write path | middleware chips on an unchanged route; empty or newline name list; unresolved reference | order unchanged, nothing sent on no-op; frozen list as today; acknowledgement gating unchanged | chips → `middlewares` | tests | sort chips |
| 12 | write path | domain already used by another route | warning only, never blocks Save or changes the rule; current route excluded | domain warning | test of warning and emitted Change | block Save |

## Acceptance cells

The proxy cells run on the Direct host (D), where the kit prepares real
Traefik and Caddy fixtures. They cross the agent path that every proxy
operation uses. X8 runs on Edge (E). The milestone matrix holds the binding
list.

- **X1.** Route list at 1440 and 390 px: tiles, badges, filter and search work,
  card rows at 390 px. Screenshots go next to the lab screenshots.
- **X2.** Create a route through Easy with a container suggestion and a
  resolver. The written file is valid, the route answers through Traefik and
  the new row shows the expected badges.
- **X3.** Edit an existing route: change only the destination. The file diff
  touches only the backend line.
- **X4.** The tooltips open on hover and on keyboard focus, and the step editor
  works at 390 px.
- **X5.** Expanded certificate details at 1440 px are not squeezed (#541).
- **X6.** The manual-import hint links to the stack editor (#503).
- **X7 (Caddy).** Editor sections and Save on a Caddy instance.
- **X8 (E).** On an Edge host, the destination suggestions list that host's
  containers.
