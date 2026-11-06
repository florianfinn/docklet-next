<!--
Source: docklet-next/issues/348
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original one-time stack import from GitHub and Docker Hub rationale.
-->

# One-time stack import from GitHub and Docker Hub

Decision for #348 (M19, maintainer kickoff 2026-11-06). Docklet is a homelab tool with normally one
administrator. A hub-only route resolves a source once and returns a draft proposal; the hub stores
nothing, the web puts the proposal into the unsaved draft, and the user reviews and deploys as usual.

## Accepted decisions (do not reopen)

- One-time import only; no permanent Git connection or Git sync ([core adoption](core-adoption.md)).
- GitHub import uses a mirror address: hub variable `STACK_IMPORT_GITHUB_RAW_URL` (default
  `https://raw.githubusercontent.com`, documented in `.env.example` as a mirror and test setting).
  The sandbox points it at a fixture server with recorded files.
- The Docker Hub tab accepts Docker Hub plus registries enabled in Settings › Registries (with their
  credentials); any other registry host is refused.
- Out of scope: private GitHub repositories, other Git hosts, files beyond the compose file and
  `.env.example`, following `include:` or `extends:` across files (reported as missing), any sync.

## Starting point (main b5068aff)

- The adapter contract exists: `importSource({kind: "github" | "dockerhub", ref}) → {title, found[],
  missing[], services[{key, body}]}` (`web/src/data/adapter.ts:69-80,137`). Only the demo implements
  it (`web/src/data/demo/import-simulator.ts`, no network). The HTTP adapter rejects it with
  `capability.mapping-unavailable`; the backend lists `importSource` without an availability case
  (`backend/internal/activity/capabilities.go:32,56-58`).
- `ImportSourceDialog.tsx` (tabs GitHub / Docker Hub, one free-text ref, Found/Missing card, Apply) is
  shown in the Easy wizard only for the demo adapter (`StackWorkspace.tsx:249,256-262`). Apply adds
  services to the draft and nothing else.
- No outbound GitHub or Docker Hub code exists. The registry service reads image configs with stored
  credentials and insecure flags of enabled registries (`internal/registry/platform.go:30-60`). The
  widget client (`internal/widget/client.go`) shows the egress limits followed here.

## Design

`POST /stack-import` (operationId `import-stack-source`, hub module `stackimport`, `agent=false`, not
under `/environments/{id}`, so never forwarded and absent from the agent contract fingerprint). Rule
`Admin` in `access/rules.go`, like create-project.

Request `{kind: "github" | "registry", ref: string ≤ 512, platform?: {os, architecture}}`. Response
`{title, composeContent, envNames[], found[], missing[], source {kind, canonicalRef, resolvedDigest?}}`.
`found` and `missing` are typed codes rendered from the catalogue, never hub text.

### GitHub

- Accepted refs: `owner/repo`, `owner/repo/path/to/dir`, `https://github.com/owner/repo`,
  `https://github.com/owner/repo/tree/<ref>/<path>`. Owner and repo follow GitHub's character set;
  path segments are percent-encoded; `..`, empty segments and backslashes are refused.
- Fetch `<base>/<owner>/<repo>/<ref or HEAD>/<path>/<file>` for `compose.yaml`, `compose.yml`,
  `docker-compose.yaml`, `docker-compose.yml` in that order, then `.env.example` (only variable names
  are kept, values are dropped). `<base>` is the mirror variable above.
- Public repositories only; no token and no GitHub API calls.

### Proposal pass (content only)

The deploy validator (`pkg/projects/load.go`) resolves the environment, `env_file`, `label_file`,
includes and extends from disk, so it is not used. `internal/stackimport/proposal.go` does its own pass:

- bytes capped at 256 KiB; YAML decoded with `yaml.v3` (alias-expansion limit built in);
- compose-go `loader.LoadWithContext` on an in-memory `ConfigDetails` with an empty environment, no
  working-directory file access and `SkipInterpolation`, `SkipInclude`, `SkipExtends`,
  `SkipResolveEnvironment`, `SkipResolveLabels` (otherwise `label_file`s are read, compose-go v2.16.1
  `loader/loader.go:704`, `types/project.go:899,912`), `SkipConsistencyCheck` (otherwise a service
  whose image comes only from an external `extends` is rejected, `loader/validate.go:30`) and
  `ResolvePaths=false`;
- YAML and schema validation stay on; the proposal's own check requires a top-level `services`
  mapping with at least one service;
- requirements are listed as `missing` codes: `env_file`, `label_file`, `include`, `extends`, relative
  `build` contexts, and `${VAR}` references without default (their names go into `envNames`).

Malformed YAML → `import.invalid-compose`; a valid file with missing requirements is a proposal, not
an error. The original bytes are returned, so unknown fields survive. Deploy keeps the strict loader.
Known limit: `configs` and `secrets` entries with a relative `file` path and absolute `build` contexts get no
`missing` code (the contract has none for them); deploy reports the absent host file.

### Docker Hub and configured registries

- `ref` is an image reference (`nginx`, `traefik/whoami:v1.11`, `registry.example.com/team/app:1`).
  The importer resolves the platform manifest through the registry path extracted from
  `PlatformImage` (credentials, insecure flag, platform matching). A registry host that is neither
  Docker Hub nor enabled in Settings › Registries is refused with `import.registry-not-configured`
  before any request.
- A config descriptor larger than 1 MiB (embedded config data included) is refused with
  `import.too-large` before fetching; the blob read is wrapped in `LimitReader(1 MiB + 1)`.
- One service is built from the config of the requested platform (default: the hub's platform):
  `image` (tag defaulted to `latest`), `restart: unless-stopped`, `ports` from `ExposedPorts`
  (published on the same port), `volumes` from `Volumes` as named volumes, and `environment` names
  from `Env` without values when the image sets no default.

### Egress limits (`backend/internal/stackimport/egress.go`)

- 10 s total budget per import, 5 s per request, at most 6 requests; 256 KiB per file. Auth and
  redirect hops count: the Docker Hub flow (ping, token, index, manifest, config blob, CDN redirect)
  uses all six and is pinned by a test. Flows that need more (an uncached ECR token with a
  multi-arch image, an insecure registry with token auth, nested indexes) fail with
  `import.source-unavailable` (known limit).
- GitHub client: HTTPS only (a validated mirror may be http on loopback or private addresses),
  redirects only to the same scheme and host and at most three, userinfo refused, proxy environment
  ignored. The registry path keeps its own transport and its legitimate auth and blob redirects.
- Hosts never come from user input except by matching a configured registry, so no private-address
  check is needed. The mirror variable changes only the base of the same request shape and is not a
  simulation switch; the hub reads and validates it at startup (agents ignore it).
- One import per user at a time (in-memory per-principal gate); a concurrent request gets 409
  `import.busy`.

### Web

- `web/src/data/http/stack-import.ts` (new, spread into the adapter) maps the response;
  `ImportResult` becomes `{title, composeContent, envNames, found, missing}`; the demo simulator
  returns the same shape.
- The dialog shows for every adapter whose `importSource` capability is available (administrator,
  hub reachable). The Docker Hub tab hint says that configured registries also work.
- Each request is bound to a generation counter plus `(kind, ref)` and the adapter identity. Editing
  the ref, switching tabs, closing or an identity change bumps the generation and clears the result;
  a response of an older generation is dropped. Apply is disabled while a request is pending and
  applies only to the draft whose workspace opened the dialog (draft key captured at open).
- Apply on a new stack replaces the draft compose with `composeContent` and adds `envNames` as empty
  `.env` lines; if the draft was already edited, a confirmation asks first. Apply never deploys. The
  stack name defaults from `title` when the name field is empty.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| Body larger than cap, endless body | cap + `import.too-large` | httptest endless body; red proof: remove the limit → red |
| Slow upstream | total budget → `import.timeout` | httptest delayed handler with injected budget; red proof: no deadline → red |
| Redirect to foreign host, scheme or loop | refused | httptest redirect to a second server; red proof: allow any redirect → red |
| Odd ref (`..`, `%2e%2e`, backslash, scp form, query) | parser refuses before any request | table test, request counter on the httptest server = 0 |
| Registry host not configured | refused before request | unknown host, request counter 0 |
| Content-only parse | no file resolution, no interpolation, original bytes | fixture with `env_file`, `label_file`, include, extends, `${REQUIRED:?x}` → proposal + missing codes; temp dir with matching files proves none was read; malformed YAML and alias bomb refused; red proof: use the `pkg/projects` loader → red |
| `label_file` and external `extends` | `import.missing.label-file` / `import.missing.extends`, original bytes | service with image and missing `label_file`, again with the file present in an observed temp dir; service whose only image source is `extends`; red proof: drop `SkipResolveLabels` or re-enable the consistency check → red |
| Config byte bound | descriptor > 1 MiB refused before fetch, read capped | registry test server with a matching-digest 2 MiB config and an embedded variant; red proof: remove the bound → red |
| Real transport at the seam | GitHub through a real `http.Client` against httptest; registry against the `registry` test server (as `platform_cache_test.go`) | as above |
| Dialog late response | generation fence | vitest: block A, then edit ref / start B / close-reopen / identity change, complete A → no Apply, no overwrite; Apply disabled while pending; red proof: drop the fence → red |
| Rights | `Admin` rule; users get 403 `access.admin-required` | rule-completeness test + handler test with a user principal |
| Hub-only | module skipped in agent and edge mode; agent fingerprint unchanged | `TestStackImportModuleHubOnly` like `TestSW10WidgetModuleHubOnly` |
| Concurrent imports | per-principal gate → `import.busy` | concurrent test with a blocking upstream |
| New codes | types, OpenAPI enum, EN/DE through the wire-code guard | guard compile + render test |

## Contract

- Route `POST /stack-import`, Admin, hub-only; request and response as above, frozen with the
  implementation step.
- `found` members: `import.found.compose`, `.env-example`, `.image-config`, `.ports`, `.volumes`,
  `.environment`. `missing` members: `import.missing.env-file`, `.label-file`, `.include`,
  `.extends`, `.build-context`, `.variables`, `.compose`.
- Errors (`types/stackimport`, OpenAPI, EN/DE): `import.invalid-reference` 400,
  `import.registry-not-configured` 400, `import.not-found` 404 (repository, path, compose file or
  image tag), `import.too-large` 413, `import.timeout` 504, `import.redirect-refused` 502,
  `import.source-unavailable` 502 (other upstream status, TLS or DNS failure), `import.invalid-compose`
  422 (parser line hint, never upstream text), `import.unauthorized` 502 (registry refused the stored
  or anonymous credential), `import.busy` 409. Upstream bodies and URLs with credentials are never
  returned or logged.
- New hub variable `STACK_IMPORT_GITHUB_RAW_URL` in `.env.example`.
- No persisted state, no migration, no agent contract change.
