# Simple users

Decision #321 for M15, accepted by the maintainer on 2026-11-01. Enforcement is described in
[resource rights](resource-rights.md); the sign-in boundary in [identity](identity.md).

## 1. Guiding constraint

A Docklet hub is normally run by one administrator. Additional users are a rare, opt-in case.
Therefore the user model is a **short allowlist on top of the administrator path**, not a second
product: nothing changes for an installation without additional users, every capability that is not
explicitly granted to users stays administrator-only, and users see a reduced interface (their
assigned stacks and their own profile) instead of filtered copies of every page.

## 2. Principals

| Principal | Source | Rights |
|---|---|---|
| Administrator | the existing global administrator account (built-in `role_admin`, global assignment) | everything, unchanged |
| User | a `users` row without the administrator role | only the allowlist in §4 for assigned applications, plus self-service (§5) |
| Machine | agent tokens, Edge tunnel credentials, environment bootstrap keys, webhook path tokens, `ADMIN_STATIC_API_KEY` | unchanged machine contracts; the static key stays an administrator credential |

Administrators are the accounts that are effective global administrators today (global `role_admin`
assignment). The administrator can promote a user to administrator and demote another administrator;
the last administrator cannot be deleted, disabled or demoted, and nobody changes their own administrator
status. Users cannot call any user-management, assignment or role operation, so self-elevation has no entry
point.

The Arcane role system stays the internal mechanism of the administrator. M15 changes no role data: the
role routes remain administrator-only API without a UI, and any non-administrator role assignment is
ignored for authorisation (a user's rights come only from application assignments). This implements the
core-adoption boundary "no granular per-feature permissions or large role editor" without a data migration.

## 3. Applications and levels

An **application** is one stack (Docklet project) on one host, identified by `(environment_id,
project_id)` with the agent-local project UUID. Standalone containers, hosts, images, volumes,
networks, backups, scans, registries, notifications, Auto Heal, events, settings and the dashboard are
not assignable and stay administrator-only. Deleting and recreating a stack with the same name creates
a new project ID; the old assignment no longer matches and is removed when the administrator next opens
the user (stale rows grant nothing).

Each assignment carries one of two fixed levels (roadmap decision, release plan M15):

| Level | Allowed on the assigned stack |
|---|---|
| Restricted | view the stack, its services and containers (detail, status, processes, stats); start (up of the unchanged definition), stop (per the hub's Stop mode), restart the stack; restart a single service; start, stop, restart a container of the stack; stack and container logs (stream and download); container shell |
| Full | everything in Restricted, plus redeploy, pull and build images, edit the Compose file, `.env` and project files, update checks, image updates and update baseline of the stack, tags, archive and unarchive |

Administrator-only even with Full: creating a stack, deleting/destroying a stack, restore points
and recovery release, recreate or edit of
standalone containers, kill/pause/commit/delete of containers, container origin assignment, and every
host-wide operation.

**Security boundary.** Full access lets a user change the Compose file of the assigned stack. A Compose
file can mount the Docker socket or host paths, run privileged containers and join any network, so a
user with Full access can obtain control of the host. Full access is a convenience boundary for trusted
people, not a security boundary against hostile users. Mount restrictions are not evaluated in M15
(excluded with this visible hint in the assignment dialog). Restricted access does not allow changing
the definition; its shell is a shell inside the stack's own containers.

## 4. What a user sees

- Navigation: Stacks and their own profile only. The landing page is the Stacks list.
- Stacks list: only assigned stacks; host shown by name. The hosts list returns only hosts that carry an
  assigned stack, with display fields only (no host maintenance, tokens or components).
- Stack page: overview, services/containers, logs, shell, and for Full the editor and updates; other
  tabs and actions are hidden; an action that exists but is not allowed by the level shows the reason
  from the capability descriptor (`access.full-required`).
- Activity: no activity centre; a user follows the progress of their own action through its run.
- Not visible: dashboard, hosts page, containers/images/volumes/networks pages, activity centre,
  events, notifications, backups, settings other than the profile.
- A foreign or unknown stack or container answers 404 `access.not-found`, identical to a missing
  resource, so existence is not revealed. Administrator-only operation classes answer 403
  `access.admin-required` (the class is public knowledge, no resource is revealed).

## 5. Self-service (every signed-in principal)

Current user, own profile and language, own avatar, own password (local mode), sign out, sign out other
sessions (local mode), personal API keys (§7), setup status (#268: the minimal setup-status read is
available to every signed-in principal; other internal settings stay hidden), capability descriptors
for resources the caller may see.

## 6. Login modes

| Mode | Administrator | Users |
|---|---|---|
| `local` (default) | existing account, unchanged | the administrator creates a user with username and initial password; first sign-in forces a password change (existing `requires_password_change`); the administrator can reset the password (new initial password, forced change), disable, enable and delete the user |
| `proxy` | **unchanged default**: if `AUTH_PROXY_ADMIN_USERS` is unset, every accepted proxy identity maps to the administrator exactly as today | only when `AUTH_PROXY_ADMIN_USERS` (comma-separated identities, startup configuration like `AUTH_MODE`) is set: listed identities map to the administrator account as today; any other identity maps to the user whose username equals the identity exactly; an identity without such a user is refused with 403 `access.not-registered`. The administrator creates proxy users by username without a password; proxy usernames may contain `@` (the proxy's identity string, no email header). No email or group headers (B5). |
| `none` | every request is the administrator, unchanged | none; Settings › Users shows a hint instead of the list; stored users and assignments persist unused |

Mode changes keep the existing rule: sessions are revoked, users and keys persist. A user created in
proxy mode has no password and cannot sign in locally until the administrator resets the password.

Disabling a user (new `users.disabled_at`): sign-in is refused (local: generic credential error),
sessions are revoked, personal API keys are rejected, proxy requests answer 403 `access.disabled`, and
open streams close (resource rights §6). Deleting a user removes assignments, sessions and keys
(foreign-key cascade) and closes streams; stored runs and activities keep their recorded actor name.

## 7. Personal API keys (E4a, #324)

Personal keys (`kind = personal`) act with their owner's current rights at request time: an
administrator key acts as administrator, a user key with the user's allowlist; a disabled or deleted
owner invalidates the key. Expiry blocks new requests; deleting a key closes the owner's open streams. Profile: create with name and optional expiry (the secret is shown once),
list with prefix, expiry and last use, revoke (delete). Creation and revocation keep requiring a browser
session (a key cannot mint keys). Following identity.md, proxy and none mode accept no user API keys; the
profile shows the section only in local mode and a hint otherwise. Scoped keys (`/api-keys` with explicit
grants) stay administrator-only: a scoped key is accepted only while its owner is the administrator.

## 8. Sign out (A12, #325)

- `local`: unchanged (revoke the session, clear cookies, go to `/login`).
- `proxy`: `POST /auth/logout` clears the hub's CSRF cookie and returns; the web then navigates to the
  logout address. The address comes from startup configuration `AUTH_PROXY_LOGOUT_URL`, validated at
  startup as an absolute `http`/`https` URL, exposed read-only in `GET /auth/mode` as `logoutUrl` in proxy
  mode and shown read-only in Settings › Sign-in. Without a configured address the action shows the hint
  "Sign out at your reverse proxy" and stays on the page. The hub cannot end the proxy's own session.
- `none`: no sign-out action (deliberate deviation from the dummy, issue #325).
