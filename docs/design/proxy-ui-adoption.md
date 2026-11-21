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
  host from the existing container inventory, with their container ports. A
  container that shares no network with the proxy container stays selectable
  and shows a warning (AUTO-7). Picking one fills scheme, host (service or
  container name) and port. The external choice keeps scheme, host and port
  fields.
- **Middlewares.** Toggle chips over the middlewares the proxy observation
  lists, each with help text by reference kind (local, plugin, internal,
  external). Help text per middleware type needs the type from the agent and is
  a follow-up (AUTO-8). Free entry stays possible for names not observed yet.
- **TLS.** Three choices that map to the existing change fields:
  - "No TLS": the entry points without TLS, and no resolver.
  - "Certificate from resolver": entry points with TLS, plus a resolver chosen
    from the inventory resolvers with their challenge summary.
  - "Keep current": the existing `PRESERVE` sentinel.

  The wildcard scope cards of the lab are left out, because they were
  simulated there (AUTO-9).
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

**#503.** The manual-import hint links to the stack editor of the stack whose
path equals the instance's source project path. Without a unique match it stays
plain text (AUTO-11; adding `projectId` to the DTO is the fallback if matching
proves unreliable).

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
| 2 | write path | TLS choice "No TLS" / "Resolver" / "Keep current" | exact `entryPoints` + `resolver` per choice; Keep sends no resolver | editor TLS mapping | one test per choice | map Keep to empty resolver |
| 3 | write path | middleware chip toggled on a name with a comma | names list keeps the exact name | chips → `middlewares` | test | split on comma |
| 4 | external data | container without ports, stopped, other host | not offered (other host, stopped) / offered without port prefill | suggestion filter | test | drop the host filter |
| 5 | external data | container not on a proxy network | offered with warning | suggestion filter | test | hide it |
| 6 | external data | rule not parseable for domain suggestions | ignored for suggestions, shown as-is in the list | domain parser | test with `HostRegexp`, compound rules | throw on parse |
| 7 | bundle | adopted code in production graph | no design-lab import, no lab strings | production boundary guard | existing guard + `check-production` | import from design-lab |
| 8 | layout | expanded certificate details at 1440 px | full-width row below the grid | `RouteList` | DOM structure test + screenshot cell | render inside the auto column |
| 9 | link | #503 hint with 0, 1, 2 matching stacks | link only for exactly one | hint component | test | link to the first match |
| 10 | step editor | narrow-screen steps hide invalid fields | Save validates all sections; errors jump to the step | editor | test | validate only the current step |

## Acceptance cells

All cells run against the real Traefik in the sandbox on the hub-local host (H).
There is one Caddy pass for the editor sections and one Edge pass for the
suggestions source.

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
