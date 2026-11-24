# Per-row inventory isolation

Decision for #600 (M38). The web maps each host's raw lists (projects,
containers, images, volumes, networks) into its own projections. Until 1.5.0,
one row that could not be mapped threw, and the reader dropped the host's whole
inventory: a stopped project with a build-only service emptied a host's stacks,
containers and network memberships, and nothing on screen explained why.

## Decision

A broken or unusual row affects only itself.

- **Build-only services are valid data.** The hub reports a service with
  `build:` and no `image:` that was never created with `image: ""` and no
  container. The project list reports it as `not-created` and the plain project
  detail as `stopped`. The web maps it as a service with `buildOnly: true`, an
  empty image and the update state `local`. There is no update check, and the
  service shows "Built locally · not created".
- **Rows are mapped one by one.** Every host projection maps its rows through a
  per-read collector (`web/src/data/http/row-isolation.ts`). A row that throws is
  left out and counted with its code and, when readable, its name. Every other
  row of the host stays. This covers the stack and container lists, the
  assigned-user stack list, images and their consumers, volumes and their
  consumers, the Docker volume supplement, networks and their attachments,
  metrics samples and navigation counts.
- **Shared context is validated first.** A project row without a string ID and
  name is never used for Compose ownership (`readableProject`). Otherwise one
  nameless project would fail the ownership lookup of every Compose container on
  the host.
- **Counts are visible.** The host observation carries
  `unreadable[kind] = {count, codes, names}`. Each reader owns the kinds it maps;
  a count of 0 clears its kind, and other kinds keep their counts. The inventory
  notice of a page names the host, the number of unreadable entries and up to
  eight names. The Hosts card and the dashboard host card append
  "N entries not readable". If no stack of the visible hosts could be read, the
  Stacks page shows "Stacks could not be read", never "No stacks yet".
- **A detail page names its own failure.** If a stack's own row cannot be mapped,
  `getStack` answers `get-project.unreadable` for administrators and assigned
  users. The page says that this stack could not be read, and nothing on it can
  act on the stack.
- **A failed read still marks the host.** HTTP and transport failures keep the
  existing host-level observation (`inventory.read-error`, `agent.*`).

Rejected alternatives: fixing only the empty image (the maintainer made isolation
the primary criterion), and placeholder stack objects in lists. Placeholders
would reach commands, the dashboard ring and search, so the notice names the
rows instead.

## Contract

No wire change. The web model gains `Service.buildOnly` and
`InventoryObservation.unreadable`. The real producer bytes come from
`backend/internal/project/build_only_web_fixture_test.go`, which writes
`web/src/data/http/testdata/build-only-project.json` with the hub's JSON options.

## Break map

| # | Row kind | Case | Decision | Test | Red mutation |
|---|---|---|---|---|---|
| 1 | external data | build-only, never-created service (producer bytes) | maps, no update check | `inventory-isolation.test.tsx` | drop the build-only branch |
| 2 | external data | one malformed project row | only that stack is left out and counted | same | map projects without the collector |
| 3 | external data | one malformed container row; navigation counts | only that container is left out | same | map containers without the collector |
| 4 | external data | one malformed image, volume or network row | only that row is left out; consumers and attachments stay | same | map each list without the collector |
| 5 | consumer | host with unreadable rows | notice, host card text, no "No stacks yet" | same (render) | drop the Stacks branch / the host card text |
| 6 | detail | the stack's own row is unreadable (administrator and assigned user) | `get-project.unreadable` | same | rethrow the mapping error |
| 7 | shared state | a reader of another kind observes the host | the other kinds keep their counts | same | replace instead of merge |
| 8 | shared context | project row without a name | other projects keep container ownership | same | use unfiltered rows for ownership |
| 9 | migration | none | — | — | — |

## Acceptance cells

M38 matrix I1, I2 (build-only project on a Direct and an Edge host) and I3
(malformed rows injected into the real hub responses at the browser boundary).
