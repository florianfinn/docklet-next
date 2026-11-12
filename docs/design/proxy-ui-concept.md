# Proxy UI concept

This local design exploration uses the existing Docklet interface and synthetic
Traefik data. It evaluates proxy management and its placement in the product.
It does not define a live proxy adapter or a release contract.

## Interaction scope

The proxy workspace opens with setup only while the current instance has no
selected container. After selection, routes are the landing view; the connection
and file/ACME configuration live under Settings. Certificates have one create
action with single-domain and wildcard choices inside the dialog. A direct
route-map tab remains available. Placement comparisons stay in an optional
expanded section.
File routes support creation, editing, duplication and deletion. Form and YAML
views operate on the same simulated route. Label routes link to the stack's
Compose editor. A container card and a stack tab provide contextual entry points;
new routes preselect the destination. Editors use centered, scrollable dialogs;
colored and titled sections distinguish domain, destination, TLS and handling.
Middleware help is available on hover or keyboard focus. Label routes provide a
readable domain/service/TLS/middleware summary with an optional raw-label view.

Setup explicitly shows the selected proxy container. Synthetic discovery offers
example Traefik containers grouped by host; manual setup accepts host,
container, dynamic configuration directory and an optional read-only API URL.
A persistent context bar shows the active instance, host and container. Its picker
separates configured instances from setup candidates and offers a new-instance
dialog. Multiple instances on the same host have distinct identities; names and
connection settings can be edited later. A container already assigned to another
instance is marked in discovery. The initial state has no selected container. Selection persists in the browser;
it does not probe Docker, connect to an API or discover real configuration.

The certificate view distinguishes automatic management from imported certificate
files. Automatic management can be configured or stopped in the simulation;
imported references can be replaced or removed. Matching is based on the server
name, including single-level wildcard coverage. A matching certificate is not an
arbitrary per-router assignment. DNS-01 is the wildcard issuance option. Resolver
forms accept a credential environment-variable name, never a secret.

Wildcard route matching and wildcard SSL are separate choices. The text example
targets Traefik v3.4 syntax: an anchored HostRegexp matches one subdomain level;
the ACME request explicitly includes the base domain and wildcard in tls.domains.
A single-host route can also request wildcard SSL. Form/YAML round trips retain
that distinction. Wildcard requests require a DNS-01 resolver in the simulation.
Reference: [Traefik v3.4 routers](https://doc.traefik.io/traefik/v3.4/routing/routers/).
Arbitrary regex rules and multi-domain TLS configurations remain outside the
prototype text editor contract.

Routes have editable, readable configuration names independent of their internal
Docklet IDs. An unsaved route suggests a name from its domain or destination;
saving freezes it. Router keys, service references and service keys use that same
name. Backend URLs continue to use the service/container hostname. Easy and
Advanced accept coordinated name changes while preserving the route identity.
Names must be unique within a proxy instance; duplicates receive numeric suffixes.
Existing browser fixtures receive names without changing their IDs or targets.
Legacy UUID-keyed YAML remains readable. The text editor rejects multiple routers,
multiple services and mismatched references rather than choosing one implicitly.

The naming break map covers fixture migration/reload, duplicate and cross-instance
names, YAML renames/references, and container identity changes. Model tests cover
these cases; browser checks cover Easy/Advanced name changes, duplicate handling,
name conflicts and persistence. Real file renames remain future adapter work.

State is local to the browser, shared across the proxy workspace, context cards
and maps, and resettable. The standard application translation and styling
systems apply. No dependency or code is adopted from Traefik Manager.

## Interface alternatives

Manager is the default. An optional comparison section retains a per-browser
switch between three complete presentations using the same store,
route operations, certificate actions and configuration. Switching layouts never
copies or transforms the user's prototype data.

- Classic retains the route table with a detail panel and the grouped dialog.
- Manager uses compact host/certificate tables, row action menus, summary counts
  and tabbed route dialogs. The maintainer's NPM screenshots are a visual and
  interaction reference only; no NPM source or operational screenshot data is
  adopted. Separate scheme/host/port inputs edit the same backend URL.
- Domain workspace uses a grouped domain index and an actionable path through
  domain, SSL and destination. Contextual edits open the corresponding step of
  a guided dialog with a live summary. Certificate coverage links back to routes.

Manager route and certificate lists reuse the stack/container overview rows,
responsive cards, hover surfaces and identical context/ellipsis action menus.
Clicking the row opens its editor (label routes open details); nested actions and
destination links keep their own behavior. Destination URLs open a separate tab.
Easy/Advanced uses the stack editor’s shared segmented toggle with form/code
icons. The route form’s sections use numbered cards with descriptions, selected
control colour and edge light, matching the stack wizard. The modes edit one
draft: switching to Advanced serializes the form, and returning to Easy parses
the YAML. Unchanged generated YAML returns to Easy even for an incomplete draft.
Invalid edited YAML stays in Advanced with its text intact. Workspace
navigation retains its control-colour underline and edge light.

All three use Docklet components, theme tokens and catalogues. SSL has one entry
selector: off, new request, or matching existing certificates. A wildcard route
supplies its own base domain; it does not ask for that value a second time. An
exact route can optionally request a wildcard certificate. Existing certificate
coverage still follows Traefik SNI rather than arbitrary certificate assignment.

## Map alternatives

- A: a separate route map in the proxy workspace, showing configured request
  paths from domain through proxy and middlewares to destination.
- B: the existing Docker topology with a supplementary route list, filtered by
  host and the selected container or network. Each route links to its details.
- C: a switch within Networks between the Docker topology and proxy route map.

The alternatives use the same data. Selecting a map node opens the route.
These are configuration diagrams, not measured connectivity or live traffic.

## Prototype boundaries

All new behavior lives under the development-only design lab. The normal
production build excludes the implementation and simulation fixtures. Production
navigation and routes do not expose the prototype. A separate local development
artifact is served through the existing development portal at its own host root.

The YAML editor supports the single-router/service shape produced by the form;
it is not a general Traefik file editor. Real configuration round trips, unknown
field preservation, concurrent external changes, DNS validation, ACME issuance,
certificate/key handling, reloads and static configuration are future adapter
work. The form's enable and redirect controls are simulated route preferences.
Expiry dates and health states are sample data. No proxy connection or certificate
operation is performed.

## Evaluation

The useful acceptance for this exploration is visible interaction: create/edit/
duplicate/delete routes, switch form/YAML, import/remove a certificate, configure
a resolver, follow container/stack links, and compare all three map placements.
The same route changes must appear in each view. Desktop and mobile layouts,
English/German catalogues, and the unchanged production exclusion are checked.
Live Direct/Edge and release acceptance do not apply to this local UI artifact.
