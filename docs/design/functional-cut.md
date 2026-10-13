# Functional cut of the imported core

The binding scope is [core adoption](core-adoption.md). Features explicitly marked
for removal there leave the API, runtime, exclusive jobs, settings and code in M1.
The PostgreSQL migration baseline was delivered in #8 after this cut, so the
cut removed obsolete model references before baseline generation. The R5
items below were assessed against the approved product scope and the imported
implementation. A retained item remains subject to later feature acceptance;
retention here does not approve its current interface or security model.

## R5 decisions

**API keys — keep.** The imported core uses API keys for agent pairing and
machine access. Optional Edge and direct multi-host operation need a non-browser
credential path. The role and key policy can be refined with the later identity
work, but removing the module in M1 would break an adopted host capability.

**Webhooks — keep.** A webhook is an automation entry point for retained
container and stack operations. The approved scope does not exclude automation,
and no replacement entry point is specified. Its token handling and action
permissions need a separate review before release.

**Search — keep.** Search spans retained resources, including stacks, images and
hosts. The approved navigation does not prohibit a search endpoint. Its result
scope can follow the new inventory structure without deleting the backend
contract during the core cut.

**Application images — keep.** Application icons and image lookup support the
retained dashboard and stack presentation. The exact artwork and source policy
remain part of frontend and asset review; the backend facility has no approved
removal decision.

**Image patching and attestations — keep.** Vulnerability scanning and full image
management are explicitly retained. Patching and attestation inspection are
adjacent operations, but the design does not decide whether they belong in the
first release. Keep their routes and records for a focused security and workflow
evaluation; do not infer removal from their absence in the scope table.

**Analytics heartbeat — remove.** The imported scheduler sends an instance
identifier and version to `checkin.getarcane.app` by default. Docklet Next is an
independently maintained product, and its approved scope does not include this
upstream telemetry destination or a replacement analytics service. The job,
setting, environment variable and metadata are exclusive to that heartbeat.
Product-facing dashboard metrics remain separate and retained.

**CLI — keep.** The imported command interface provides local administration,
OpenAPI export and Edge certificate generation. These support retained operation
and the checked-in contract. Remove only subcommands exclusive to cut features.

**Registry browsing — keep.** Image management and one-time DockerHub import
remain in scope. The registry browser provides metadata and selection for those
workflows. Its credentials and service access should be evaluated with the
import flow, not discarded in this cut.

**Environment deployment snippets — keep.** Host onboarding explicitly calls
for a Compose command, downloadable Compose file and copyable installer command.
The existing snippets are a source for those outputs; remove Swarm-specific
snippet generation with Swarm while keeping direct and Edge host deployment
paths.

## Database ownership

The [fresh PostgreSQL baseline](database-baseline.md) replaces the imported
Arcane migration chain. Issue #8 generated it from retained models and schema
integrity properties; it omits Swarm, GitOps, Git repositories, templates,
central variables, S3 destinations, build workshop records, OIDC, passkeys,
federated credentials, APNS and SQLite artefacts. No earlier Arcane database
is imported.

## Francis storage

Francis v0.1.0 links its SQLite provider into both binaries through the
`host/local` import. Docklet Next therefore keeps the required Francis packages
under `third_party/francis/` with the same module identity at revision
`23cd46831a543a73eec05ad034afd46245225876`. The local subset has
PostgreSQL and in-memory providers; SQLite providers and migrations are removed.
`scripts/provenance.json` records each file's source and deviation. The MIT
licence and the Apache-2.0 notices for adopted Dapr code remain intact.

The default test suite needs no database service. PostgreSQL persistence tests
use an explicitly configured integration environment. PostgreSQL system backup
creation, listing, recovery and scheduled policy runs currently return
unavailable; [issue #32](https://github.com/florianfinn/docklet-next/issues/32)
owns implementation of this gap. Local volume backups remain available.

The Docker info response embeds the Docker SDK's `system.Info`, including its
Swarm status and nested OpenAPI schemas. This is daemon information passed
through by the retained Docker info route; no Swarm management operation remains.
