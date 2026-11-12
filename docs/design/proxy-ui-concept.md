# Proxy UI concept

This local design exploration uses the existing Docklet interface and synthetic
Traefik and Caddy data. It evaluates proxy management and its placement in the product.
It does not define a live proxy adapter or a release contract.

## Interaction scope

The proxy workspace opens with setup only while the current instance has no
selected container. After selection, routes are the landing view; the connection
and file/ACME configuration live under Settings. Certificates have one create
action with single-domain and wildcard choices inside the dialog. A direct
route-map tab remains available. Placement comparisons stay in an optional
expanded section.
File routes support creation, editing, duplication and deletion. Form and proxy-specific text
views operate on the same simulated route. Label routes link to the stack's
Compose editor. A container card and a stack tab provide contextual entry points;
new routes preselect the destination. Editors use centered, scrollable dialogs;
colored and titled sections distinguish domain, destination, TLS and handling.
Middleware help is available on hover or keyboard focus. Label routes provide a
readable domain/service/TLS/middleware summary with an optional raw-label view.

Setup explicitly shows the selected proxy container. Synthetic discovery offers
example Traefik and Caddy containers grouped by host; manual setup accepts host,
container, dynamic configuration directory and an optional read-only API URL.
A persistent context bar shows the active instance, host and container. Its picker
separates configured instances from setup candidates and offers a new-instance
dialog. Multiple instances on the same host have distinct identities; names and
connection settings can be edited later. A container already assigned to another
instance is marked in discovery. The initial state has no selected container. Selection persists in the browser;
synthetic setup does not probe Docker or discover real configuration. Optional lab candidates use the development bridge described below.

The certificate view distinguishes automatic management from imported certificate
files. Automatic management can be configured or stopped in the simulation;
imported references can be replaced or removed. Matching is based on the server
name, including single-level wildcard coverage. A matching certificate is not an
arbitrary per-router assignment. DNS-01 is the wildcard issuance option. Resolver
forms accept a credential environment-variable name, never a secret.

Wildcard route matching and wildcard SSL are separate choices. The text example
targets Traefik v3.4 syntax: an anchored HostRegexp matches one subdomain level;
the ACME request explicitly includes the base domain and wildcard in tls.domains.
A single-host route can also request wildcard SSL. Form/text round trips retain
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
the proxy-specific text. Unchanged generated text returns to Easy even for an incomplete draft.
Invalid edited text stays in Advanced with its text intact. Workspace
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

## Caddy mapping and limits

Each instance owns its proxy kind. Synthetic discovery derives it from the image;
manual setup chooses it explicitly. Existing version-1 browser instances migrate
to Traefik without changing IDs, names, routes or targets. Caddy has file routes
and ACME issuers; plain Caddy does not have Docker label routes.

Easy and Advanced share the same draft. Caddy Advanced uses a single Caddyfile
site block, starting with `# docklet-route: <name>`. The parser consumes only this
form grammar and rejects unknown directives, additional sites, ambiguous address
lists and unsupported upstreams. Route IDs never become Caddy configuration names.

| Form | Caddyfile |
| --- | --- |
| Domain / HTTP only | Domain site address / `http://domain` |
| Wildcard matching | Literal `*.example.com`, no regular expression |
| Path prefix | Named `path /prefix*` matcher and `handle`; keeps the prefix |
| Ordered middlewares | `route` block: compress → `encode gzip`; strip-api → `uri strip_prefix /api`; custom-headers → `header X-Proxy demo` |
| Destination(s) | `reverse_proxy` with HTTP(S) upstreams; multiple destinations add `lb_policy round_robin` |
| Automatic HTTPS | Default Caddy automation; configured DNS-01 adds `tls { dns <provider> {env.<CREDENTIAL>} }` |
| Imported certificate | `tls <certificate-file> <key-file>` from a matching imported reference |

`handle_path` would strip a path implicitly, unlike the form's PathPrefix
contract. The explicit matcher preserves the path; only strip-api removes `/api`.
Upstreams cannot contain credentials, a base path or a query. Unknown middlewares
are unavailable and require explicit removal, never a silent omission. Caddy DNS
issuance requires a build containing the chosen provider module. Imported file
references do not upload files to the proxy.

Wildcard certificate scope remains separate from route matching. An explicit
certificate-domain comment retains the form choice. For automatic wildcard SSL,
the one site block also lists the base domain and wildcard as certificate subjects;
a host matcher restricts forwarding to the route domain. This also keeps an exact
route exact when requesting wildcard SSL. Caddy 2.10 prefers applicable managed
wildcards; there is no Traefik-style per-router TLS SAN field. See the official
[Caddy automatic HTTPS](https://caddyserver.com/docs/automatic-https) and
[common patterns](https://caddyserver.com/docs/caddyfile/patterns) documentation.
Enable and redirect remain simulated preferences; applying a lab route uses the
shown proxy configuration, including Caddy's default automatic HTTPS redirects.

## Development lab bridge

An optional local development bridge served same-origin under `/lab-api/` exposes
runtime proxy discovery, capabilities, suggested hostnames and backend URLs. A
failed or unavailable info request hides lab controls quietly. No lab hostnames,
addresses or environment credentials are compiled into the prototype.

Only instances created from lab discovery are linked. Settings show reachability,
version, reload method and capabilities, including unavailable public ACME and DNS
modules. Save keeps a browser draft. Save & apply to lab also sends generated
Caddyfile or dynamic Traefik YAML to the bridge with host, path and TLS probes;
multiple backends request four samples. Advanced apply may submit invalid text to
obtain a real proxy error while preserving the last valid saved draft. Responses
show policy/adapt/load stage errors, snippet lines, warnings, router/service errors
and a probe table with HTTP status, backend, certificate issuer and redirect.

Route rows report applied, error or not applied and offer a new-tab link only for
runtime lab hostnames. Observations are memory-only and become unknown after a
page reload; they are not a permanent health claim. Editing generated text makes
a previous observation stale. Rename applies the new name before deleting the old
one. Delete removes the lab name before removing the browser draft; failures stay
visible for retry. Settings can re-apply all file routes for this instance. The
bridge owns proxy reload semantics and may retain an earlier active configuration
after a failure. Attempted route names persist with the browser draft so a
failed rename can still be cleaned up after reload; observations themselves do not
persist. Applied names must be deleted before unlinking the lab connection.
Cancelling an unsaved applied draft removes its lab names first. Synthetic and production instances never use this bridge.

The lab uses local/default TLS certificates and cannot demonstrate public ACME.
Missing Caddy DNS modules surface as real adapt errors. A Traefik API success does
not validate an unknown certificate resolver. Certificate collection operations
still describe simulated management, not live issuance or file transfer.

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
Expiry dates and health states are sample data. Synthetic instances perform no proxy or certificate operation. Lab-linked instances explicitly apply configuration through the development bridge; certificate collection actions remain simulated.

## Evaluation

The useful acceptance for this exploration is visible interaction: create/edit/
duplicate/delete routes, switch form/YAML, import/remove a certificate, configure
a resolver, follow container/stack links, and compare all three map placements.
The same route changes must appear in each view. Desktop and mobile layouts,
English/German catalogues, and the unchanged production exclusion are checked.
Live Direct/Edge and release acceptance do not apply to this local UI artifact.
