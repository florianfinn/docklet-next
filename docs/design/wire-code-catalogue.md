<!--
Source: docklet-next/issues/357
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original wire codes in the web catalogue rationale.
-->

# Wire codes in the web catalogue

Decision for #357 (M19, maintainer kickoff 2026-11-06). Docklet is a homelab tool with normally one
administrator; the guard exists so that a German session never shows English or raw wire text. It
lands first in M19, before the steps that add new codes.

## Accepted decisions (do not reopen)

- The guard is the first M19 step; later steps add EN and DE keys in the same commit as their codes.
- No backend, contract or migration change.

## Starting point (main b5068aff)

- German sessions showed English or raw wire text in M17 (`notification.send`, hub event titles,
  `volume.backup.interrupted`) and M18 (`job.*` names, scheduler states, "Update check: partial").
  Each time a code from the API contract reached the UI without a catalogue key, the renderer fell
  back to the hub's English `sourceText` or to the code itself (`web/src/data/message-code.ts:49,53`).
- Only hub event titles have a build-time guard
  (`HUB_EVENT_CODES satisfies Record<HubEventCode, true>`, `web/src/data/http/map-dto.ts:168`).
- German parity already follows from `catalogs: Record<AppLocale, Messages>`
  (`web/src/app/i18n/catalog.ts:5-7`).

## Design

Every string-literal union in the checked-in contract that the web renders as text has a total map
from each member to a catalogue key, checked by the TypeScript compiler. A missing member fails
`tsc`; a key that does not exist in the English catalogue fails `tsc`.

- One module `web/src/data/wire-catalogue.ts` (split per family above ~400 lines) exports, per
  rendered union `U`, `const X_KEYS = { ... } satisfies Record<U, CatalogueKey>`. `U` comes from
  `components["schemas"][...]["code"]` in `web/src/api/generated/schema.ts`. Optional generated
  properties are typed `NonNullable<…>`. `CatalogueKey` is use-intl's
  `MessageKeys<Messages, NestedKeyOf<Messages>>`, the type `t()` accepts.
- Renderers read the key from the map instead of building it from the code string. The `sourceText`
  fallback stays for codes outside the contract (older stored rows, newer agents); it never applies
  to a member of a guarded union.
- Families whose members share a key builder (for example `settings.codes.${reason}`) may map through
  that builder; the map still lists every member, so the compiler checks totality.
- `knownBackupError` (`web/src/data/http/map-dto.ts:308`) consumes the total backup map, so backup
  codes on job rows are covered.

### Closed completeness criterion

1. **Pinned set.** A test reads `api/openapi.json` and collects every schema property named `code`,
   `reason`, `errorCode`, `messageCode`, `baselineCode`, `pointCode`, `policyCode`, `status`,
   `state`, `kind`, `outcome` or `phase` whose schema has a string `enum`. Each `(schema, property)`
   appears exactly once, either in the guarded list (with its map) or in the exemption list with a
   one-line reason (for example "transport only, never rendered"). A new enum property in the
   contract fails the test until it is classified.
2. **Totality.** Each guarded map is `satisfies Record<U, CatalogueKey>` (compile time).
3. **Render use.** For each guarded union one test renders one member through the real renderer in
   `de` and asserts the German catalogue text, not the English source text.
4. Plain `string` fields (activity `type` and `status`, scheduler step names and statuses, job and
   resource names, `Activity.error`, metadata paths, `EnvironmentEnvironment.status`) are outside
   the guard. They keep their existing known-list tests (for example the backup job-row and
   `activity.jobStep.*` regression tests), and the language cell of practical acceptance covers them.

Bypass forms outside these four points are nonblockers for a follow-up, not verification failures.

### Classification at the time of writing

Guarded: shared message codes (123, of which 75 are already guarded event titles), updater reasons
(110), `BackupProblem.code` (63) and `.reason` (11), `ImageProblem.code` (61), `ImagePruneItem.code`
(52), `VulnerabilityProblem.code` (26), scan `errorCode` (23), `EnvironmentHostProblem.code` (13),
`WidgetProblem.code` (11), `NetworkError.code` (11), `ContainerProblem.code` (10),
`NotificationProblem` (6), `UserAccessProblem` (6), notification dispatch `kind` (7), scan, backup,
restore and step statuses, `AgentRestartRun.state` and update mode. Four message codes have no key
(`runtime.wait-timeout`, `runtime.waiter-disconnected`, `runtime.target-removed`, `run.fresh`); the
guard step adds EN and DE text for them and for any other gap the compiler reports.

### Consequence for later steps

Every step that adds an enum member to a rendered union (import errors, notification target codes,
`container_unhealthy`, cooldown and validity codes) adds EN and DE keys in the same commit; the build
enforces it. Language checks in the candidate and acceptance runs then look at layout and wording,
not at missing keys, for enum-coded text.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| New contract enum member without key | compile error | red proof: add a member to one map's union in a test-local type copy → `tsc` fails (recorded) |
| New enum property in the contract unclassified | pinned-set test fails | red proof: drop one entry from the guarded list → test red |
| Renderer bypasses the map (builds the key from the string) | render test per union in `de` | red proof: revert one renderer to `sourceText` → test red |
| Plain-string fields | stay with known-list tests | existing regression tests kept |

## Contract

No route, DTO, error code, migration or agent contract change. The only product change is catalogue
text (EN and DE) for the four message codes above and any further gap the compiler reports.
