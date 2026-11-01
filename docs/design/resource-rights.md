# Resource rights enforcement

Owner issue #322 (with #255, #268, #275, #276). Product model in
[simple users](simple-users.md). Foundation of M15, built on main `6365cc5cebd04f199d15fe5f53c006a127d0a669`.

## 1. Principle

Docklet is a self-hosted homelab tool. Normally one administrator controls everything; additional users
are a rare, low-priority case. The administrator path stays exactly the existing Arcane permission path.
A **user** takes a separate, small path: an explicit rule per operation, default deny, hiding rather than
filtering, and no extra safeguards for unlikely situations. Rare races and duplicate-name situations are
documented limits (§10), not additional mechanisms. All decisions stay on the hub; agents remain
unauthorised executors behind the hub, and the agent contract does not change.

## 2. Data (migration 015, single owner: S1)

- `users.disabled_at DATETIME NULL`.
- `user_app_assignments(id TEXT PK, user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  environment_id TEXT NOT NULL, project_id TEXT NOT NULL, level TEXT NOT NULL CHECK (level IN
  ('restricted','full')), created_at, UNIQUE(user_id, environment_id, project_id))`. Removing a host removes
  its rows (foreign key where the environment row exists, else the host-removal path deletes them).
- No role data changes: roles, role assignments and the static admin key stay as they are. The migration
  set also runs on agent databases; the new table stays empty there.

## 3. Principal

- **Administrator** = today's effective global administrator (`PermissionSet.IsGlobalAdmin()`, sudo, none
  mode, proxy identities per simple-users §6, the static admin key). Nothing changes for this path.
- **User** = every other signed-in account. Its rights come **only** from `user_app_assignments`; any
  legacy non-admin role assignment is ignored (not deleted), so no granular Arcane permission reaches a
  user. Promotion and demotion use the existing global `role_admin` assignment.
- For users the `PermissionSet` is synthesised: `Global` = self-service permissions; `PerEnv[env]` = the
  union of the level permission sets (§4) over the user's assignments on `env`, including the supporting
  grants those operations check internally (for Full: `images:pull`, `image-updates:read/check`,
  `activities:read`). The access rule (§5) always runs first, so a synthesised grant never authorises an
  operation outside the allowlist.
- Each user has a `rights_revision` counter (in-memory per process is sufficient: it only triggers stream
  closure, §7). Mutations invalidate the user's token, role and key caches after commit.
- A disabled user fails authentication (401 for sessions and keys, 403 `access.disabled` in proxy mode).
  Ordinary account writes (last login, profile, password) never write `disabled_at` (the whole-user
  update omits it), so they cannot undo a disable.

## 4. Rule table

`backend/internal/access/rules.go` holds one entry per operation: every Huma operation ID of the exported
OpenAPI document and every non-Huma route (Echo WebSockets in `api/ws/proxy_permissions.go`, `/stream`,
diagnostics streams, `/debug/pprof`, uploads). Kinds:

| Kind | Meaning for a user |
|---|---|
| `Public` | unchanged public contract |
| `Machine` | not reachable with a user credential (agent, Edge, pairing, webhook, notification dispatch) |
| `Self` | allowed; acts on the caller only |
| `Admin` | 403 `access.admin-required` before the handler (default for everything not listed) |
| `App{Level, Ref}` | allowed only if `Ref` resolves to an assigned stack with at least `Level` |
| `AppList` | allowed; the response contains only assigned items |
| `OwnRun` | allowed only for runtime runs the caller started (persisted caller ID) |

Allowlist (exact operation IDs; S1 confirms them against the stack page's real calls):

- Restricted, `ProjectPath` (`{projectId}`): `get-project`, `get-project-runtime`, `deploy-project` (Start:
  see options rule below), `stop-project`, `down-project` (configured Stop mode), `restart-project` (stack or
  single service), WebSocket `projects/:projectId/logs`.
- Restricted, `ContainerPath` (`{containerId}`): `get-container`, `get-container-processes`,
  `start-container`, `stop-container`, `restart-container`, `download-container-logs`, WebSockets
  `containers/:id/logs`, `containers/:id/stats`, `containers/:id/terminal`.
- Full, `ProjectPath`: `get-project-compose`, `update-project`, `redeploy-project`, `pull-project-images`,
  `build-project-images`, `update-project-services`, `get-project-updates`, `get-project-workspace`,
  `update-project-workspace`, `get-project-workspace-file`, `download-project-workspace-file`,
  `update-project-tag`, `archive-project`, `unarchive-project`, `get-update-baseline`,
  `check-project-updates`, `update-project-images`.
- Administrator-only (also for Full): restore points (`list-restore-points`, `delete-restore-point`,
  `restore-update`, host restore points), `release-runtime-run` (administrator recovery acknowledgement),
  stack create/delete/destroy, every host-wide operation.
- `AppList`: `list-projects`, `listEnvironments`.
- `OwnRun`: `get-hub-runtime-run`, `cancel-hub-runtime-run` (and the env-0 equivalents
  `get-runtime-run-outcome`, `cancel-runtime-run`). The activity list and activity detail routes stay
  administrator-only; a user follows their own action through the runtime run.
- `Self`: `get-current-user` (extended with `isAdmin` and the caller's assignments), `update-my-profile`,
  avatar, `change-password`, `logout`, `logout-all-other-sessions`, `list/create/delete-my-api-key`,
  `get-capabilities`, the setup-status read (#268).
- `/stream`: users receive only the `environments` channel, projected (§6); `activities`, `events`,
  `dashboard` are not delivered to users.

**Restricted Start options.** For a Restricted caller `deploy-project` accepts only an empty or default
options body (Compose default pull behaviour: images are pulled only when missing). Any `pullPolicy`,
`forceRecreate`, `removeOrphans` or `recreateVolumes` value from a Restricted caller answers 403
`access.full-required` before admission. Full and administrator callers keep every option.

The table is the only place where user rights are declared; adding a user operation later is a one-line
table change plus its test row.

## 5. Enforcement points (one decision function)

`access.Decide(ctx, principal, rule, envID, params) → Allow | NotFound | AdminRequired | FullRequired`.
Order: kind → assignment of the resolved stack → level/options. An unassigned and a missing resource give
the same `NotFound`.

| Point | M15 change (administrators unchanged) |
|---|---|
| All Huma operations (any registration form) | one router-level Huma middleware after authentication looks up the operation ID and calls `Decide` for users before operation middleware runs |
| Echo WebSockets, diagnostics, pprof (hub-local) | `RequireEchoPermission` and the diagnostics/pprof guards call `Decide` for users (diagnostics and pprof are `Admin`) |
| Remote environments (Direct and Edge), incl. the vulnerability branch | both `proxyPermissionDenied` call sites call `Decide` with the operation ID from the matcher, before feature veto, shell acceptance, snapshot read and agent-token injection |
| Hub management paths for remote hosts | served by Huma, so covered by the router-level middleware |
| Capability descriptors | `Decide` without side effects per represented action and resource; level too low → `available=false`, `access.full-required`; invisible resource → 404 |
| `/stream` | users get only the projected `environments` channel |

## 6. Resource resolution and lists

- `ProjectPath`: `(envID, projectId)` must be an assignment row; no host read is needed.
- `ContainerPath`: the container is allowed if its canonical ID appears in the runtime view
  (`get-project-runtime`, the same producer the stack page uses) of an assigned project on that host. The
  hub obtains that view with agent authority (local service call for the hub host, the normal Direct HTTP /
  Edge request path for remote hosts), decoded into a fresh value, never through the inventory snapshot.
  The path's container reference is matched against the canonical ID and the names in that view. A read
  failure answers 503 `access.unavailable`; absence answers 404.
- `list-projects` for a user: the hub reads the host's complete project list with agent authority (fresh
  value, never written to the snapshot or a shared flight), keeps assigned project IDs, then applies the
  caller's search, sort and pagination and computes totals from the filtered set. Snapshot and coalesced
  results are never modified.
- `listEnvironments` and the `environments` SSE channel for a user: only hosts with an assignment, each
  projected to `{id, name, status, colour}` on a subscriber-owned copy. The administrator payload is
  unchanged.
- Web (S3): for a user the stack list comes from `list-projects` and services/containers from
  `get-project-runtime`; the web does not call `list-containers` and a Restricted stack page does not fetch
  the Compose file.

## 7. Revocation of open streams

- `sessionlife` gains a binding that does not require a browser session ID: every user principal (local
  session, proxy identity, personal key) attaches a lease keyed by user ID at the three authentication entry
  paths before stream acceptance; real session IDs are kept for existing session logout semantics.
  The lease validator fails when the user is gone, disabled or its `rights_revision` changed.
- Every user mutation (assignment change, level change, disable, delete, password reset, promotion,
  demotion, key delete) commits, bumps the revision, invalidates caches and calls
  `sessionlife.Revoke("", userID, "")`. This closes that user's hub-local WebSockets, Direct and Edge
  proxied WebSockets and terminals and SSE (terminals through their existing bounded close handshake).
  Clients reconnect and are decided again.
- Personal-key expiry blocks new requests; an already open stream ends at its next validation or close.
- Accepted commands and their NDJSON progress continue. Administrators are unaffected.

## 8. Follow-ups folded in

- #255: the agent registry sync receiver requires the create and delete grants for those effects (tested
  through its real authenticated receiver; the route is agent-only).
- #268: setup status becomes a `Self` read; other internal settings stay filtered.
- #275: user-originated scan requests (including the administrator's own) are forced to `origin: manual`
  in the local handler and the remote canonicalisation; hub-dispatched scans keep `scheduled`.
- #276: the embedded `vulnerabilityScan` summary is removed from image list responses.

## 9. New codes

`access.not-found` (404), `access.admin-required` (403), `access.full-required` (403),
`access.disabled` (403), `access.not-registered` (403), `access.unavailable` (503): `types/`, OpenAPI
enums, generated DTOs, web reason mapping, EN/DE catalogues, contract fingerprint.

## 10. Limits (documented, no extra mechanism)

- Two Docklet stacks with the same Compose project name on one host: the runtime view of either may list
  the other's containers; Docklet already refuses runtime actions for such duplicates.
- A request already forwarded before a revocation completes; only streams are cut.
- Full access is not a security boundary (simple-users §3).

## 11. Break map

Seams are tested once per class through the real transport (Direct httptest agent and the real Edge tunnel),
not per operation.

| Row | Decision | Entry boundary | Test (red mutation) |
|---|---|---|---|
| RR1 | every operation (OpenAPI IDs + non-Huma routes, hub and agent route sets) has exactly one rule; stale keys fail | rule table vs exported spec and registered routes | completeness test; mutation: drop one entry / add a ghost entry |
| RR2 | user on every `Admin` operation → 403 before the handler | router-level Huma middleware, Echo guards, environment middleware | all operations through the real router with a user principal; mutation: skip `Decide` for bare `huma.Register` |
| RR3 | foreign or missing project → identical 404 on hub-local, Direct, Edge | `Decide` `ProjectPath` | one real Direct + one real Edge case; mutation: 403 for unassigned |
| RR4 | container in the assigned stack's runtime view allowed; standalone or other-stack container → 404; read failure → 503 | resolver | hub-local + real Direct + real Edge; mutation: allow on read error |
| RR5 | Restricted: Full operations and non-default Start options → 403 `access.full-required` after assignment check | `Decide` | mutation: drop the options check; swap order |
| RR6 | `list-projects` filtered before pagination with filtered totals; `listEnvironments` and SSE `environments` projected | AppList | assigned item on a later upstream page, zero assignments; mutation: filter after pagination / skip SSE projection |
| RR7 | the user list and projection paths never read from or write to shared read results: the inventory snapshot for `list-projects`, the shared environment values of `listEnvironments`/SSE | AppList on fresh values, projection on subscriber copies | `-race` with concurrent administrator and user callers on the real snapshot producer and the real SSE producer; assertion: an administrator read and an administrator subscriber after/alongside user reads see the complete unprojected data; mutations: write the user-filtered body into the snapshot; project the shared environment value in place |
| RR8 | users read and cancel only their own runtime runs; `release-runtime-run` stays administrator-only; no `activities`/`events`/`dashboard` channel for users | OwnRun, stream | mutation: compare host instead of caller |
| RR9 | user mutations close the user's open log, shell, stats WebSockets (hub-local, Direct, Edge) and SSE; the next request is decided on current rights | revision + revoke + cache purge | cookie, proxy-identity and personal-key principals on real streams; mutation: no revoke / no cache purge / no lease for sessionless principals |
| RR10 | 015 forward on hub and agent DBs from an M14 database; administrator, roles and static key unchanged | migration | upgrade test on an M14 fixture; mutation: 015 touches role tables |
| RR11 | the last administrator cannot be deleted, disabled or demoted; users cannot reach user, assignment or role operations; an ordinary account write after a disable keeps the user disabled | user service | mutation: drop the in-transaction admin count; write `disabled_at` in the whole-user update |
| RR12 | proxy mapping per simple-users §6 | `ModeRequestUser` | mutation: case-insensitive match; fallback to administrator for unknown |
| RR13 | personal key acts with the owner's current rights; disabled or deleted owner → rejected; scoped key of a non-administrator → rejected; proxy/none reject user keys | key resolution | mutation: resolve with cached administrator set |
| RR14 | six codes in types, OpenAPI, DTO, fingerprint, EN/DE, web mapping | contract | existing guards; mutation: drop a DE key |
| RR15 | synthesised user `PerEnv` = allowlist permissions + named supporting grants; a Full update admission succeeds | synthesis | positive Full update test; mutation: remove `images:pull` |
| RR16 | `AUTH_PROXY_LOGOUT_URL` absolute http(s), validated at startup; `logoutUrl` only in proxy mode; logout keeps Origin + CSRF | config, `/auth/mode`, logout | mutation: accept `javascript:` |
| RR17 | #255, #268, #275, #276 per §8 | respective routes | one red proof each |
| RR18 | a stack deleted and recreated with the same name is not granted by the old assignment; containers recreated by a deploy stay accessible | ProjectPath / ContainerPath | mutation: match by project name |
