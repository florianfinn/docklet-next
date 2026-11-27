# Proxy file metadata on replacement (#663)

Design for M44 (#663). Extends the [proxy module](proxy-module.md) rows
"Metadata" and "Write, project source".

## Decision

The agent keeps the atomic publish (staging file in the target directory,
rename onto the target). It no longer refuses a file because it carries
extended attributes. It refuses only what a replacement would lose, and the
operator can accept exactly the reported loss for one operation.

### Loss rule (one primitive)

`hostio.ProjectWriter.Prepare(from, to)` opens both entries on the pinned
directory descriptor with `O_NOFOLLOW`, applies the final mode to the staging
file (`to`'s current mode when it exists, otherwise the requested mode; ACL
masks follow the mode, so the comparison only happens after this chmod), and
returns a `Loss` computed on the two open descriptors:

- Every extended attribute of the target that the staging file lacks or holds
  with a different value is lost (`Attributes`: sorted names).
- A target with `nlink > 1` loses its other links (`Links = nlink - 1`; they keep
  the old bytes).
- Attributes only the staging file has (the directory's default ACL on a target
  that had none) are not a loss. A new target loses nothing.
- `system.posix_acl_access` is compared as parsed entries, not bytes. The
  owner, mask and other entries mirror the mode bits the writer sets on
  purpose (the proxy keeps the target's mode; the resolver tightens a changed
  `.env` to 0600), so they are ignored; named user and group entries and the
  owning-group entry are compared by tag, id and permissions. A malformed ACL
  value refuses (`unsupported-metadata`). Every other attribute is compared by
  value.
- `Token`: hex SHA-256 over the canonical loss (sorted name/value pairs of the
  lost attributes, `nlink`, target dev/inode). Raw values never leave the agent.
- Reading: `flistxattr`/`fgetxattr` with a bounded buffer (64 attributes,
  64 KiB in total); a size change between the two calls is retried once, then
  refused. Only `ENOTSUP`/`EOPNOTSUPP` mean "no attributes"; any other error
  (permission, I/O, `ERANGE` after retry, over the bound) refuses with
  `unsupported-metadata` as today.

The staging file itself is the agent's own new file; its inherited attributes
are accepted. Type, link count of the staging file (must be 1), and the
existing mode rules (`Mode&0044`, `Mode&07000`, owner-only refusal) stay.

### Where the rule runs

- **Staging check (report).** In phase `staging`, after `Stage` and `Validate`,
  the agent calls `Prepare`. A non-empty loss whose `Token` is not in the
  request's acknowledgement ends the operation `rejected` with the new reason
  `metadata-loss` and the structured `metadataLoss` list. The phase never
  reaches `publishing`; nothing is published.
- **Publish check (authoritative).** `Replace` calls `Prepare` again
  immediately before `Renameat`. Then it re-reads both names with `fstatat`
  and requires the same dev/inode as the open descriptors (no entry swap). A
  different token than the acknowledged one, or a swapped entry, refuses the
  rename; the operation ends `rejected` with `metadata-loss` (no effect).
- **Residual race.** The source lock serialises Docklet operations only. A
  foreign process can still change attributes between the final check and the
  rename (a window of one syscall); this is stated in the design doc.

### Paths that replace nothing

- **Remove** (restore of a created file; cleanup of reserved names): the inode is
  unlinked, nothing is carried over. Type and confinement checks stay; no
  attribute comparison.
- **Caddy delete publish** (`Replace(Target → TempPath)`): moves the original
  inode; no attribute comparison.
- **Caddy delete rollback** (new): when the target is absent and the reserved
  temporary entry still holds the original bytes (hash equals `Old`), restore
  renames it back with `RENAME_NOREPLACE`, which keeps the original inode and its
  attributes. Only when that entry is gone does restore fall back to writing the
  old bytes as a new file (attributes of the lost inode cannot be recovered; the
  restore receipt says so with `metadata-loss` details).

### Resolver files (maintainer Q-2: in scope, no force)

The DNS-provider resolver (`resolver/files.go`) reads and replaces `.env` and
Compose files of the proxy stack with its own staging and rename.

- **Read** (`readAt`) no longer refuses a file for carrying extended
  attributes; reading loses nothing. The mode rules stay (regular file,
  `nlink == 1`, no special bits, no group/other write; with an ACL the group
  bits are the mask, so no named entry can write either).
- **Replace** computes the loss with the same comparison on the staging file
  after its final chmod (forward write: `WriteMode`, restore: original `Mode`)
  and the target, immediately before the rename, after the existing hash
  recheck. Any loss refuses with `unsupported-metadata` and the list in the
  diagnostic detail; there is no acknowledgement. Confidentiality: the 0600
  write mode sets the ACL mask to none, so inherited named entries grant no
  access to the written `.env`.
- Resolver restore uses the same rule; a foreign attribute added after the
  forward write ends `recovery-required` as today.

### Restore

Restore replaces the published target with the old bytes through the same
`Replace`, with **no** acknowledgement. After a confirmed publish the target
holds the directory's attributes and so does the new staging file, so the loss
is empty and restore proceeds. If a foreign process added attributes since the
publish (same bytes), the loss is non-empty and restore ends
`recovery-required`, as any unsafe restore does today.

Rejected alternatives:
- In-place writes (Arcane `acfs` `InPlace`): keep all metadata, but Traefik
  watches the directory and can read a torn file, and the recovery protocol
  relies on an atomic publish.
- Atomic without any check (Arcane `atomic.WriteFile`): silently drops a per-file
  ACL.
- Copying attributes onto the staging file: SELinux labels and privileged
  namespaces need capabilities the agent may not have; not needed for the
  common inherited case.
- Reporting the loss in Read or Inventory: needs a host-side stat or a probe
  file in the operator's directory during a read, and stays a stale snapshot.
- Acknowledging names only: a changed value under the same name would pass;
  the token binds the confirmation to the observed snapshot.

## Contract

Owner: step S1 (one owning step).

- `types/proxy`:
  - `MetadataLoss{Path string; Attributes []string; Links int; Token string}`.
  - `Acknowledgements.MetadataLoss []MetadataLoss` with `json:",omitzero"`.
  - `Operation.MetadataLoss []MetadataLoss` (`omitzero`) on a `metadata-loss`
    rejection and on a restore receipt that could not keep attributes.
  - Canonical form: the hub and the agent normalise an empty list to nil
    before hashing, persisting and sending (`normaliseRequest` used by
    `Digest`, `Hub.Apply` and `Agent.Apply`), so `[]`, `null` and absence give
    the same digest and the same wire bytes as a request from main.
  - Validation (`validRequest`): at most 16 entries; each path is the request
    target; no duplicates; `Links` 0–1024; names ≤ 255 bytes, printable;
    `Token` 64 hex. Host-produced lists obey the same bounds.
  - New reason `metadata-loss`: `state.go` enums, `outcomes.json`, OpenAPI,
    generated schema, `proxy-decoder.ts` reasons, wire catalogue, en/de.
- Agent↔hub contract **v25**: `agent-hub-v25.json`, `AgentHubVersion = 25`,
  frozen `AgentHubV24Fingerprint`, v24 branches in `AgentCompatible` and
  `ModuleCompatible`; current-version fixtures and tests that name v24
  (`types/contract/*_test.go`, `backend/api/*_test.go`, `tests/*-contract*.mjs`,
  `tests/openapi-drift.test.mjs`), `docs/design/api-contract.md` version table.
  `MinimumAgentVersion` unchanged.
- **New hub, agent below v25 (v18–v24):** the hub refuses a request with a
  non-empty acknowledgement in `CreateOperation`, before admission and before any
  record or reservation, with the existing `agent.update-required` module error
  (lookup: the environment's current `AgentContractVersion`, as
  `activity/runtime_sender.go` does). `Hub.Apply` repeats the check before
  dispatch. Environment `0` (in-process agent) is always current. Without an
  acknowledgement nothing changes: the field is absent, the old agent answers
  as today (`unsupported-metadata`).
- **Old hub, new agent:** unsupported upgrade order (agents never update past
  the hub, `docs/compatibility.md`). A v24 web decodes `metadata-loss` as an
  invalid response. Documented in the upgrade notes; no negotiation.

## State and lifecycle

- The staging check sits inside phase `staging`; a `metadata-loss` rejection
  there has no effect and the existing staging cleanup removes the staging
  file. The receipt with `metadataLoss` is persisted in the agent journal and
  returned by Query; the hub stores it in the operation record.
- Crash after staging, before publishing: startup recovery sees an unchanged
  target and rejects as today; cleanup removes the staging file.
- Crash during or after a confirmed publish: existing recovery; restore follows
  the Restore rule above (no acknowledgement; foreign additions →
  `recovery-required`).
- The acknowledgement is part of `ApplyRequest`, the request digest and the
  journaled intent; replay with a different acknowledgement is
  `operation-conflict`. Hub records and agent journals written by main (no
  field) keep their digests.

## Break map

| # | Row kind | Case | Decision | Entry boundary | Test | Red mutation |
|---|---|---|---|---|---|---|
| 1 | external data | default-ACL directory, target with the inherited ACL | no loss, accepted | `Prepare` | hostio, raw `system.posix_acl_default` with named entry + mask on `t.TempDir` | refuse any staging xattr → assertion fails (`unsupported-metadata`) |
| 2 | external data | target with a differing per-file ACL | `rejected` `metadata-loss` in phase `staging`, list + token; target bytes and xattrs unchanged | agent staging check | agent test with real `ProjectWriter`; asserts phase never `publishing` | remove staging check → phase `publishing` observed |
| 3 | external data | target with 2 links | loss `Links: 1`; confirmed → published, other link keeps old bytes | `Prepare`, `Replace` | hostio + agent | ignore `nlink` → no rejection |
| 4 | external data | named-user ACL whose mask changes with mode; target mode changes between staging and publish | comparison after final chmod; changed token → refuse | `Prepare` | hostio | compare before chmod → false loss reported |
| 5 | external data | same names, changed value after confirmation | token mismatch → rename refused, `metadata-loss` | `Replace` | hostio with value change between calls | compare names only → publish happens |
| 6 | persisted transitional | entry swap of target or staging name between check and rename | refused, no effect | `Replace` dev/inode recheck | hostio with deterministic swap hook | drop recheck → swapped inode renamed |
| 7 | persisted transitional | restore after confirmed publish; foreign same-byte xattr added before restore | equal → restored; added → `recovery-required` | `Runtime.Restore` | agent recovery with real writer, incl. startup recovery | skip comparison in restore → foreign ACL lost, status `restored` |
| 8 | persisted transitional | Caddy delete then rollback | original inode renamed back, attributes kept; fallback receipt lists loss | `Runtime.Restore` | caddy agent test with ACL on the deleted file | rewrite old bytes instead → xattr missing |
| 9 | persisted transitional | restore of a created file in an ACL directory | removed, `restored` | `Remove` | agent recovery | re-enable xattr refusal in `Remove` → `recovery-required` |
| 10 | new code | `metadata-loss`, `metadataLoss` | producer → journal → Query → hub record → decoder | real agent producer | bootstrap test with real agent in ACL temp dir, Direct and Edge; decoder contract test | omit `metadataLoss` from the Query receipt → assertion fails |
| 11 | transport parity | acknowledgement Direct vs Edge | identical body and result | `proxyWireFixture` | real Direct and Edge | drop field in Edge command body → mismatch |
| 12 | version | non-empty acknowledgement to v18–v24 agents | `agent.update-required` before admission, no record | `CreateOperation`, `Hub.Apply` | hub tests per version, Direct and Edge, no dispatch recorded | remove gate → dispatch observed |
| 13 | migration-like | journaled main requests; nil vs `[]` acknowledgement | same digest and bytes | `normaliseRequest`, `Digest` | golden digests of main fixtures on hub and agent | drop normalisation → digest differs for `[]` |
| 14 | external data | `ENOTSUP` empty; `EACCES`/`EIO`/size race/over bound refuse | as stated | attribute reader | hostio with injected reader errors | map all errors to empty → assertion fails |
| 15 | external data | malformed acknowledgement (count, path, name, token, duplicates) | `invalid-input` | `validRequest` | engine validation table | remove a bound → accepted |
| 16 | consumer | Traefik marker write in an ACL directory | `Mark` succeeds, marker cleaned up | `traefik/observe.go` | real `Mark` with real writer | refuse staging xattr → `Mark` fails |
| 17 | consumer | Caddy staging directory and files in an ACL directory; root import insertion | unaffected by the rule (new files) | `caddy/staging.go` | caddy test in ACL temp dir | refuse inherited xattr → staging fails |
| 18 | external data | SELinux-like label equal / different | equal → accepted; different → loss | attribute reader | injected reader (no SELinux in CI) | ignore `security.*` → no loss |
| 19 | old hub | v24 decoder vs v25 receipt | documented unsupported; `invalid-response` | web decoder | v24 decoder fixture test asserting the documented result | — (documents behaviour) |
| 20 | external data | resolver read of a `.env` with an inherited ACL | read succeeds; mode rules unchanged | `resolver.readAt` | resolver files test in ACL temp dir | restore `attrs != 0` refusal → read fails |
| 21 | external data | resolver forward write tightening an ACL file to 0600 | named entries equal → replaced; written file grants no named access (mask none) | `resolver.Replace` | resolver files test asserting parsed ACL and effective mask | compare raw ACL bytes → refused |
| 22 | external data | resolver target with a differing named entry | refused `unsupported-metadata`, list in detail, target unchanged | `resolver.Replace` | resolver files test | skip comparison → replaced |
| 23 | persisted transitional | resolver restore after forward write; foreign attribute added | equal → restored; added → `recovery-required` | resolver recovery | resolver recovery test with real files | skip comparison on restore → restored |
| 24 | external data | malformed `system.posix_acl_access` value | refused `unsupported-metadata` | ACL parser | hostio parser table | accept malformed → compared as bytes |

## Kickoff decisions (2026-11-27)

1. **Report point:** first Apply (Q-1 a). #663 text updated.
2. **Resolver files:** in scope, same comparison, no force (Q-2 b).
3. **ENOTSUP:** "no attributes" only for genuine non-support (main thread).
4. **Approvals:** feature-branch pushes, draft closing PR, squash merge, fix
   PRs after acceptance, record, sandbox, follow-up issues; no release, tag or
   deploy (Q-3 a).

## Acceptance cells

Sandbox D (Direct) and E (Edge), Traefik instance with a project-source dynamic
directory from the kit. H is excluded by environment limit: the kit prepares no
proxy fixture on H, and H runs the same agent code in process (covered by unit
and bootstrap tests). Methods were checked in a baseline run on the main package before the milestone.

- C1 (D, E): default-ACL directory: create and edit a route without
  acknowledgement; verified; files keep the inherited ACL. Baseline on main:
  `unsupported-metadata`.
- C2 (D, E): target with a differing per-file ACL: first Apply rejected
  `metadata-loss` listing the attribute, bytes and attributes unchanged;
  confirmed retry verified; warning names the permanent loss.
- C3 (D): hard-linked target: rejection names one extra link; confirmed retry
  verified; the other link keeps the old bytes.
- C4 (D): single-file mount stays read-only.
- C5 (D): a confirmed publish that Traefik rejects (method from baseline) is
  restored without a new prompt; a created route in the ACL directory restores
  by removal.
- C6 (D): directory without ACLs: create and edit unchanged.
- C7 (D): resolver: proxy stack `.env` with an inherited ACL in the
  default-ACL project directory; a resolver change applies, the `.env` ends
  0600 with no effective named access; a `.env` with a differing named entry
  is refused with the list and stays unchanged. Method: the resolver fixture of
  the M28 acceptance in the kit. If the kit cannot run a resolver change on D,
  the runner reports C7 as an environment limit; rows 20–23 then carry the
  evidence and the verdict does not depend on C7.
