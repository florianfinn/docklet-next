# First-run setup and stored sign-in policy

M29 (#518) revises the mode selection of decision #68 ([identity](identity.md)).

## Decision

First-run setup is the single entry point of a fresh hub.

- It chooses the sign-in mode (`local`, `proxy` or `none`) and creates the first
  administrator with a username of the operator's choice, in every mode.
- `local` adds a password. There is no built-in account, no initial password in the
  log and no forced password change.
- The mode is a stored setting. **Settings** › **Sign-in** changes it without a
  restart.
- A non-empty `AUTH_MODE` is a forced override. It fixes the mode and, for `proxy`,
  also takes the header and trusted networks from the environment. Setup and
  Settings show the mode locked, and policy writes are refused.

Rejected alternatives:

- A temporary setup session that runs the whole wizard before any user exists. Every
  settings and discovery read assumes a request user.
- Keeping the built-in account and renaming it in setup. That leaves a guessable
  credential reachable until setup finishes.
- Selecting the mode only through the environment. The maintainer wants it chosen in
  setup and changeable later.

## Setup phases

1. **Identity claim** (public, `POST /api/setup/identity`). The request carries the
   setup code, the mode, the proxy header and trusted proxies (proxy mode only), the
   username and, for `local`, the password. On success the hub signs the browser in:
   local mode sets a session cookie, while proxy and none need no session.
2. **Configuration wizard** (authenticated, unchanged). It covers addresses,
   projects, modules and review, and Finish sets `setupCompleted=true`.

`GET /api/auth/mode` reports two flags that the web gate uses:

- `setupPending`, while the claim is open;
- `forced`, when `AUTH_MODE` fixes the mode.

While the claim is pending, the same endpoint also returns the effective proxy
header and trusted proxies for display. Administrators read them later through
`GET /api/auth/sign-in`.

## Setup code

- Each hub start that finds the claim open creates a code: 12 characters from
  `crypto/rand` in an unambiguous alphabet, shown as `XXXX-XXXX-XXXX`.
- The code is logged once and kept only in memory. Comparison ignores case and
  dashes and runs in constant time.
- The code becomes invalid when the claim commits. A restart before the claim
  replaces it.
- After 10 failed attempts within a rolling minute the endpoint answers 429. The
  claim route also shares the per-IP limit of login and refresh.

## Claim states

The internal setting `setupIdentity` records the claim. No row means pending, and
`done` means claimed. Upgraded 1.0.0 hubs have no row, so they open setup again;
no SQL migration is needed.

While the claim is pending:

- Non-public REST operations, Echo streams and environment forwarding answer
  401 `setup.pending`.
- Local login, refresh and password change answer 401 `setup.pending`.
- Session, Bearer and personal API key verification fail, so old sessions stop.

This applies in every mode, including a forced `none`. Public operations (mode,
health, version, pairing, avatars, app images, the claim itself) keep their
contracts. Independently validated machine ingress also keeps working: environment
access tokens and the Edge tunnel. `AUTH_RESET_ADMIN_PASSWORD` is consumed and
ignored while the claim is pending.

The claim runs in one write transaction:

1. Re-check `setupIdentity`.
2. Create the administrator, or take over the oldest enabled global administrator
   (otherwise the oldest disabled one, which is re-enabled), keeping its ID.
3. Set username, display name and password hash, and clear
   `requires_password_change`.
4. Grant the global Admin role if it is missing.
5. Store the complete effective policy triple (mode, header, trusted list); when
   forced, that is the forced values.
6. Write `setupIdentity=done` and `setupCompleted=false`.
7. Revoke all sessions and record `authLastMode`.

Only after commit does the hub publish the new policy, clear the token and
permission caches, refresh the settings cache, reconcile the static admin API key
(a failure only logs and is retried at the next start) and, in local mode, issue
the session.

Concurrent claims serialise on the policy mutex and the transaction. The second
claim sees `done` and gets 409 `setup.not-pending`. A username held by another
account is refused with `user.username-taken`.

## Policy publication

`AuthService` holds one immutable `Policy` snapshot (mode, header, networks,
forced, pending), and every request reads it once.

- Readers that used `Config.SignInMode()` now read the snapshot or the user
  service's mode source. This covers the CSRF header-credential exemption, user
  creation, the password reset and the environment-proxy identity header.
- Writers hold a mutex from transaction begin through publication. A failed commit
  publishes nothing.
- With `AUTH_MODE` set, the effective policy is always the environment view
  combined with the published claim state.

Removing `AUTH_MODE` resumes the stored triple. The claim recorded the forced
values, so removing the variable never changes the mode silently. Startup revokes
sessions when the mode differs from `authLastMode`, as before. An invalid stored
policy fails startup unless `AUTH_RESET_ADMIN_PASSWORD` is set without a forced
mode; that recovery stores local sign-in with the new password (see
[identity](identity.md#operator-recovery)).

## Runtime mode change

`PUT /api/auth/sign-in` is restricted to global administrators and is refused with
`auth.mode-forced` while forced. A change applies only if the caller stays signed in
afterwards:

- **proxy**: under the proposed policy, this request must come from a trusted
  direct peer with exactly one valid identity. With `AUTH_PROXY_ADMIN_USERS` set,
  the identity must be listed or equal the caller's username. Otherwise the request
  fails with `auth.proxy-unverified`.
- **local**: the caller has a password or sets one in the same request. Otherwise
  the request fails with `auth.password-required`.
- **none**: always allowed; the UI warns.

Every change revokes all sessions in its transaction. In local mode the caller
receives a fresh session in the response.

## Break map (tests in `backend/internal/auth/setup_test.go`)

| # | Case | Guard | Red proof |
|---|---|---|---|
| 1 | claim recheck inside the transaction | `setupIdentity` read in the tx | marker check disabled → late claim creates a user |
| 2 | concurrent claims | mutex + tx re-check | one success, one `setup.not-pending`, one user |
| 3 | wrong / brute-forced code | constant-time compare, 10 per minute | limiter disabled → 11th attempt not 429 |
| 4 | forced mode | `auth.mode-forced`, forced triple stored at claim | check removed → stored mode differs; triple not stored → mode flips on env removal |
| 5 | upgraded hub | takeover keeps the ID; old sessions and password refused | takeover disabled → new user |
| 6 | pending gate (REST, stream, login) | `Policy.Pending` in identity and local checks | gate removed → forced none opens; local check ignores pending → old token valid |
| 7 | runtime change | revoke in tx, reissue in local | revoke removed → old cookie still valid |
| 8 | lockout guard | principal under proposed policy | peer or caller check removed → change accepted |
| 9 | rollback | publish after commit | publish before write → policy diverges from the database |
| 10 | strict claim CSRF | no transport or header-credential exemption on `/api/setup/` | strict flag off → transport token bypasses Origin |
