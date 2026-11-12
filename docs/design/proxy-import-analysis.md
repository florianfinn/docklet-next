# Proxy import analysis

Dated evidence for the proxy module, which is planned after M26. On 2026-11-12 one
real, maintained Traefik edge installation was inspected read-only. Its dynamic
configuration was copied without secret files. This note records structural
findings only. Host names, router names, addresses and the copy stay private.

## Installation shape

- Traefik v3.7.16 with the file provider and watch enabled. The Docker label
  provider and the API are off. Three entry points: public HTTP, public HTTPS and
  an internal HTTPS entry point bound to a private interface. Forwarded headers
  and PROXY protocol trust one upstream address.
- One DNS-01 certificate resolver (Cloudflare) against the Let's Encrypt
  production directory. ACME storage lives in a separate volume. A plugin
  middleware (CrowdSec bouncer) is loaded through static configuration.
- One shared dynamic file holds 11 routers, 7 services and 6 middlewares. About
  40% of its lines are comments. Two further files hold secret middleware
  definitions, with checked-in `.example` templates next to them.

## Findings that constrain the module

| Finding | Consequence |
| --- | --- |
| All routes share one file | The module edits entries inside an existing file. Comments, ordering and unrelated entries must survive byte-for-byte outside the edited node. One file per route is only the default for new routes. |
| Heavy comments | Use a comment-preserving YAML document model. A parse/serialize round trip of a whole file is not acceptable. |
| Middlewares referenced from secret files | A reference to a middleware defined elsewhere is valid. It is shown as external, never as an error. Secret files (`*.secret.yml`) are never read, displayed or rewritten. Their names are matched by pattern, not by a fixed extension. |
| One router requests the wildcard certificate (`tls.domains`); the others use `tls: {}` | Certificate coverage comes from the certificate store and SNI, as the prototype models. The module shows which router requests a certificate and which ones only rely on it. |
| Certificate resolvers exist only in static configuration | The UI selects existing resolvers. Creating or changing one is a static change that needs a Traefik restart, and is out of scope for the first module version. |
| Services shared by up to three routers | A route is not always a 1:1 router/service pair. Editing a shared service shows every dependent router. Deleting a route keeps a service that other routers still use. |
| `priority`, `Path` versus `PathPrefix`, combined host/path rules, a catch-all redirect router with a no-op service | The form handles the common single-host shape. Other rules stay editable as text and are preserved unchanged. Catch-all and redirect routers are recognised as infrastructure, not as application routes. |
| Middleware types: headers, redirectScheme, forwardAuth, plugin | The middleware list comes from the real configuration, including plugins. The prototype's fixed list is only demo data. |
| No API | Router state cannot be read back. Verification uses a pre-write syntax and reference check, a post-write probe and, where available, the Traefik log. |
| Separate internal entry point | Entry points come from the installation. A route may target an internal entry point that is not publicly reachable. |

## Issuance

Certificate issuance is a function of Traefik or Caddy. The module writes correct
configuration and shows the outcome. It does not reimplement or separately test
ACME. Acceptance checks the written configuration, the served certificate and the
reported issuance errors against a real installation.

## Classifier result (2026-11-12)

The development-only classifier (`traefik-import.ts`) reads the private copy
without writing to it. It finds 11 routers: 3 editable in the form, 7 text-only
and 1 infrastructure. The text-only reasons are shared services, `priority`, a
combined rule and a service defined in a secret file. Middleware references
resolve as local, plugin or external, and none counts as an error.

Applying a no-op edit to every router leaves the file byte-identical, 11 of 11.
Targeted edits patch only value spans of the Document source. A synthetic fixture
with the same shape covers comments and unrelated bytes. YAML anchors, aliases,
tags and removals from compact flow maps are refused explicitly; the file is never
reserialized as a whole. Removing a route keeps every service, because files that
cannot be read may still reference it.

Recommended direction, to be confirmed at module kickoff: imported file, router and service identities
are preserved and changed through targeted patches. Routes on shared services stay
text-only in the first version, and resolvers are selected, never created.

## Scoped write test (2026-11-12)

The test ran with operator approval for this session. It first ran on the lab
Traefik, which uses the identical image digest, and then on the real edge. On the
edge the dynamic directory was backed up first. The test wrote one new file,
generated by `addRouteToFile`, with a single router: a host covered by the
existing wildcard certificate, the public HTTPS entry point, a middleware
defined in the existing shared file, `tls: {}` and `noop@internal`. The file was
written to a temporary name without a YAML extension and then renamed.

- Traefik loaded the file without a restart; the router answered after about
  4 s. The served certificate was the existing wildcard from the certificate
  store, and the cross-file middleware headers were present.
- Removing the file rolled the change back within about 2 s. Existing routers
  returned unchanged status codes before, during and after the test. The shared
  file's checksum stayed the same, and the Traefik log had no warnings or errors.
- The test exercised the edge entry point locally on the host. The public
  upstream path in front of the edge was not part of it.

Lab-only, deliberately not tried on the edge: one invalid YAML file in the
directory freezes the whole file provider. Existing routes keep serving, but no
later change from any file applies. After a Traefik restart in that state, every
route is gone. The module therefore has to validate the complete text before
writing (syntax and references). It writes atomically through a temporary
non-YAML name, never leaves an invalid file in place, and checks the directory
for invalid files before it writes.

Two module gaps found: `addRouteToFile` rejects a new file that holds only a
comment, and additions to a flow-style map stay on one flow line. Creating a new
route file needs a block-style skeleton.

The resulting module design is in [proxy module](proxy-module.md).

## Next evidence

1. With separate operator approval for each session: a further scoped write test on the
   real installation in its own file, with a backup first and removal as rollback.
