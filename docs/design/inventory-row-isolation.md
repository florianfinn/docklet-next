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
  consumers, the Docker volume supplement and bind metadata, network summaries,
  networks and their attachments, metrics and navigation counts.
- **Shared context is validated first.** A context row is used only when its
  identity can be represented: a non-empty string ID without `~`
  (`representableId`, the same rule as `inventoryId`). A project row also needs a
  string name (`readableProject`). An unreadable project never wins Compose
  ownership, so it cannot fail every Compose container on the host. A network
  summary without such an ID is left out and counted before any detail read, and
  a container without one is never matched as a dependency target.
- **Every mapper checks its own row first.** A row's identity must be
  representable and the name it displays must be non-empty text; otherwise the
  row is unreadable. This holds for list rows and for stack details of
  administrators and assigned users alike.
- **References are entries of their own.** Inside a row, every entry that points
  to another object (an image's consumer containers and `usedBy` entries, a
  volume's consumers and Docker usage names, each attachment of a container to a
  network) is read one
  by one. An entry that cannot be read is left out and counted under the reader's
  kind; the row and its other entries stay. The row's own data (ports, mounts,
  network settings, a stack's runtime services) failing makes the row unreadable.
- **Detail pages have one boundary.** Once the host has answered, any failure of
  a stack detail (an empty or mismatched detail, a projection error) is
  `get-project.unreadable` for administrators and assigned users; so is a list
  read the host answered with a body or pages Docklet cannot read. Only a missing
  requested list row is `get-project.not-found`; transport, host and maintenance
  failures keep their outcomes.
- **One physical row counts once.** A reader counts a row under its kind once,
  even when several of its parts fail (for example a network's summary and its
  inspected detail). A network whose detail cannot be read still shows from its
  summary and is counted.
- **Counts are visible.** The host observation carries
  `unreadable[kind] = {count, codes, names}`. A count of 0 clears its kind, and
  other kinds keep their counts.
- **Each kind has owning readers.** Only a reader that reads the kind's complete
  host list publishes that kind, and its count covers every row it left out:
  - `stacks`: the stack/container list and the assigned-user stack list;
  - `containers`: the stack/container list and the metrics container list;
  - `images`: the image reader, including a consumer container it tried to map
    and could not;
  - `volumes`: the volume readers, including consumer containers, the Docker
    volume supplement and bind metadata rows;
  - `networks`: the topology, including summaries, details and containers it
    could not place.

  A consumer container counts under the reader's own kind, never under
  `containers`. Partial reads isolate rows but publish no count, so they cannot
  clear an owner's count: the label-filtered container list of a stack detail,
  the network summary list, single volume and network details and navigation
  (which publishes no host observation). A readable container whose metrics
  sample is invalid has no sample, like a container without one; it is not an
  unreadable row. The inventory
  notice of a page names the host, the number of unreadable entries and up to
  eight names. The Hosts card and the dashboard host card append
  "N entries not readable". If no stack of the visible hosts could be read, the
  Stacks page shows "Stacks could not be read", never "No stacks yet".
- **A detail page names its own failure.** If a stack's own list row has no
  readable identity, or it or its runtime entries cannot be mapped,
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
| 9 | shared context | project ID with `~` competing for ownership | valid owner keeps its containers and volumes | same | accept any non-empty ID |
| 10 | external data | null network summary (topology, summary list) | other networks and attachments stay; counted by the topology only | same | read details of unvalidated summaries |
| 11 | detail | unrelated null project; requested row without name or with malformed runtime entries | detail readable / `get-project.unreadable` | same | unguarded find, no readable check, unguarded runtime scan |
| 12 | external data | null bind metadata row | every volume stays; counted under volumes | same | map bind rows without the collector |
| 13 | shared state | consumer, placement and metrics failures | counted under the owning kind; recovery clears | same | discard the collector / invalid sample counted |
| 14 | shared context | dependency target with `~` in its id | healthy dependencies stay | same | match unvalidated targets |
| 15 | detail | stack detail name not text (administrator, assigned user) | `get-project.unreadable` | same | truthy name check |
| 16 | shared state | malformed network detail; summary and detail of one row both fail | counted once under networks | same | drop the detail count / the row key |
| 17 | external data | `usedBy` / Docker usage entry that cannot be read | image or volume and other consumers stay; counted once | same | map entries without the collector |
| 18 | detail | empty stack detail (administrator, assigned user) | `get-project.unreadable` | same | check outside the boundary |
| 19 | external data | one attachment that cannot be read | the container's other attachments and dependencies stay | same | one collector for all attachments |
| 20 | detail | malformed successful list read; transport failure of the list read | unreadable; own outcome | same | empty batch (not-found) / everything unreadable |
| 21 | migration | none | — | — | — |

## Acceptance cells

M38 matrix I1, I2 (build-only project on a Direct and an Edge host) and I3
(malformed rows injected into the real hub responses at the browser boundary).
