<!--
Source: approved Docklet design prototype (maintainer's own work)/react-opus
Revision: d75474089b87ab9f77c606cc7aa696d05157d8e9cd902f1ed5a0f2b904722a39
Retrieved: 2026-10-13
License: Apache-2.0 AND MIT AND BSD-3-Clause
Deviations: Historical import proof with subsequent product boundary, English catalogue base, adapter port and validation guards documented below.
-->

# Frozen UI snapshot provenance

Source: approved Docklet design prototype (maintainer's own work), `react-opus`,
frozen remediation state L4 dated 2026-10-12; imported on 2026-10-13 for issue #10.
The source is a local design prototype without a Git revision. Content hashes
identify the accepted snapshot; no source-project history is imported.

Own code is Apache-2.0. Third-party components retain the licences and origins
below; this record does not replace their notices. Existing source notices are
preserved byte-for-byte. The source `PROVENANCE.md` content SHA-256 is
`568e79d447c5bff8ca51b80d6888d6e0286dd4d435b935a9db4d8d5a255c24d9`.

## Snapshot identity and reproducible proof

At the M2-01 import, `web/src/` contained **592 files**. Its SHA-256 aggregate was
`d75474089b87ab9f77c606cc7aa696d05157d8e9cd902f1ed5a0f2b904722a39`.
Each imported file was byte-identical to the frozen source, including catalogues, comments,
styles, tests and the vendor provenance record. There are no added source headers.
Exact sidecars in `scripts/provenance.json` bind every imported file to its source
content hash and current bytes; the snapshot entry records the aggregate.

From either the prototype root or `web/`, run:

```sh
find src -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum
find src -type f | wc -l
```

The aggregate hashes the byte-sorted list of `<sha256>  <path>\n` with `src/`
prefixes, exactly as the source `designbook/scripts/snapshot.mjs` does.
This is the initial import proof, not a prohibition on separately approved later
product changes. The frozen reference JavaScript bundle is `index-DZjVeqYz.js`;
its SHA-256 is `fef415bcd35709ee7fbe1dd1e3c340c3d9f4a914ed8e4e667616f640ef776497`.
The reference CSS bundle is `index-BD8sSDoW.css`. Built output is not imported.

## Imported files and exact deviations

Copied: `src/`, `public/`, `index.html`, `eslint.config.js`, `tsconfig*.json`,
`vite.config.ts`, `vitest.config.ts`, `components.json`, `package.json`,
`pnpm-lock.yaml` and `pnpm-workspace.yaml`.

At the initial import the only changed file was `package.json`: its name is `docklet-next-web`
instead of the prototype package name. All dependency versions, scripts and
other package fields were unchanged. The lockfile and workspace settings were
byte-identical, including the reviewed esbuild build permission.

`web/` remains a self-contained pnpm package/workspace with its own lockfile.
Use `pnpm --dir web install --frozen-lockfile`. Existing root lint, test and build
commands already delegate to this package; existing change-scope rules already
classify all `web/` changes as executable. No guard script changes are needed.
CI enables pinned pnpm 11.21.0 and installs the web lockfile before running root
lint (ESLint plus TypeScript), tests and build. The existing SHA-pinned actions,
read-only permissions, text-only fast path and 15-minute timeout are retained.

No installed dependency tree, build output, lab documentation tree, design book,
environment files, screenshots, videos, renders or prototype README are copied. This new
provenance record and repository policy entries are import metadata.
The generated lockfile alone has an exact size-policy exemption; no source
file receives an oversized-file allowance.

## Third-party and first-party origins

- **shadcn/ui, MIT:** `src/shared/ui/` primitives and `src/shared/hooks/use-mobile.ts`
  originate from the official free `new-york` Tailwind v4 registry, installed with
  `shadcn@4.21.4` on 2026-10-10. There is no registry Git revision in the prototype;
  per-file snapshot hashes identify the imported versions, not untouched registry
  originals. Existing adaptations cover import aliases, theme-independent sonner,
  the external-store mobile hook, deterministic sidebar skeletons, control/hover
  colour tokens, `BareTable`, toast sizing/lifetime, slider thumb accessibility,
  fade-only popup/sheet transitions, toggle state colour and localized spinner
  labels. The context menu is an own Radix wrapper using the dropdown's MIT styles.
- **Fonts, OFL-1.1:** Fontsource variable Montserrat and Geist Mono, both pinned to
  5.3.0, are package dependencies imported by `src/styles/globals.css`. No font
  binary is checked into this snapshot. Preserve their OFL texts in distribution.
- **React Flow, MIT:** `@xyflow/react` 12.12.0 remains the topology runtime
  dependency. Its visible attribution is retained without source modification.
- **OKLab:** `src/shared/theme/color.ts` uses published matrix constants from
  Björn Ottosson, "A perceptual color space for image processing", checked by the
  prototype on 2026-10-10 (public domain, alternatively MIT), and WCAG 2.2 relative
  luminance/contrast formulas. The prototype records no copied code beyond those
  constants/formulas. Source: `bottosson.github.io/posts/oklab/`.
- **Arcane, BSD-3-Clause:** design reference `getarcaneapp/arcane` v2.15.1,
  `095cb778daba1034e2a39961199e712a75d4ba29`, retrieved 2026-10-10. The stylesheet
  token basis is `frontend/src/routes/layout.css`; the update hover panel follows
  `frontend/src/lib/components/image-update-item.svelte` structure/colours without
  digests. Runtime palette derivation and design controls are prototype changes.
  No Svelte code was copied. The root Arcane licence and NOTICE remain applicable.
- **Earlier own mockup, Apache-2.0:** Compose model/operations/import parsing and
  demo import simulation adapt
  `handoff/opus-pc/src/app/variants/opus-arcane/compose.ts`, copied 2026-10-10 without
  an upstream commit. Existing headers explain their adaptations.
- **Logo renderer, Apache-2.0:** the twelve unchanged JavaScript modules in
  `src/shared/brand/vendor/` originate from the maintainer's `logo-next/src/`,
  retrieved 2026-10-11. Their source paths and twelve SHA-256 values remain in
  [the vendor record](src/shared/brand/vendor/PROVENANCE.md), are checked by
  `brand.test.tsx`, and are repeated as exact asset allowlist entries. Four type
  declarations and that record are also unchanged. The public favicon is an own
  geometric drawing; service-widget marks are simplified own drawings, not copied
  third-party logos.

The package and unchanged lockfile pin the prototype dependency graph. Runtime
libraries include React, React DOM, React Router, TanStack Query, use-intl,
Radix UI, lucide-react, sonner, class-variance-authority, clsx, tailwind-merge,
Recharts, yaml, CodeMirror and Lezer; the prototype records compatible
MIT/ISC/Apache-2.0 terms apart from OFL fonts. This snapshot record is not a
complete transitive dependency licence inventory or release licence bundle;
those remain M2/release integration duties from the repository licensing policy.

## Free-only and publication review

`components.json` has no custom `registries` entry or premium source. The imported
`free-only.test.ts` checks all TS/TSX/CSS sources for premium imports, registry
references and the removed runtime component-set switch, and checks static
routing to the free implementations. A separate source search includes JavaScript
vendor modules. Premium-related text occurs only in the exclusion test and this
provenance explanation, not in a premium implementation.

The exact proposal command is:

```sh
node scripts/publication-allowlist.mjs --propose web/
```

Every proposed entry was reviewed with its source line and context. Exact line
hashes, occurrence counts and specific public reasons are recorded in
`scripts/publication-allowlist.json`; no directory is exempt.

- `literal-secret`: 71 exact entries, 73 occurrences.
- `network-address`: 17 exact entries, 25 occurrences.
- `private-hostname`: 22 exact entries, 24 occurrences.
- `workstation-path`: 33 exact entries, 47 occurrences.

No filename findings, actual private data or usable credentials were found.
Synthetic network exceptions cover parser policy boundaries, malformed inputs,
link-local discovery filtering and an invented bridge-address translation test;
they do not describe an operating network. Other exceptions cover public UI
labels, template substitutions, dummy test inputs, generic demo installation
paths, code member access and a package version. The favicon is geometric SVG
without external content; the twelve logo modules generate own vector artwork.
There are no checked-in font binaries or other media under `src/`.

This section records the approved prototype snapshot. The product boundary and
English catalogue changes after the import are documented below. HTTP/WebSocket
integration and live product acceptance belong to later steps.

## Product boundary after the import

Issue #11 relocates design tokens and hooks to `shared/theme`, and favourite
codes, their controls, explanation preferences and shared catalogues to
`shared/preferences`. Exact per-file sidecars retain the original source path
and hash while recording the current hash and deviation. Removed source paths
have no stale sidecar. The aggregate above remains the historical import proof.
Existing catalogue bytes, storage keys and version 1 records are preserved.

`src/test/fixtures/lab-preferences-v1.json` contains nine synthetic localStorage
records captured by executing the pre-move stores at commit
`3d449aa` with non-default preferences. It includes a favourite encoded by that
implementation. Regression tests load these records through the product stores
and require an identical design-code export.

The build selects `src/runtime.ts` (disconnected product adapter) or
`src/runtime.dev.ts` (development server and tests). All distributable builds,
including custom modes, use the product entry. No URL or browser preference
selects a runtime. The login simulator, lab controls, simulation badges and demo
translation tables stay behind this boundary. Setup writes fail honestly until
a backend is connected; personal appearance and help settings remain available.

Root lint and test build and inspect the complete production module graph.
`build/production-boundary.ts` rejects demo and lab modules, including modules
removed by tree shaking. It emits `production-modules.json` with repository-relative
graph and shipped module paths. Root tests additionally inject forbidden imports
into the real application in memory and require the guard to reject them.
The normal build uses the same guard. No HTTP adapter is implemented here.

## English catalogue base and continuing validation

The M2-01 import commit is `ce68e4343dbf5b774bcdeb93d85036ea158e61ab`.
On 2026-10-13, all **46 imported catalogue files (23 DE/EN pairs)** matched their
prototype source SHA-256, and the **21** pre-English German file hashes embedded
in the imported catalogue test matched this import. The table below records
those import bytes, including the locale switch and L1a address-pool additions.
Hash each complete UTF-8 file with SHA-256; paths are relative to `web/`.
These are historical evidence, not ongoing byte-freeze checks.

| Imported catalogue | SHA-256 |
| --- | --- |
| `src/app/i18n/messages/de.ts` | `f668505599823d17eb14500abdfde255863ee4a9b571dbb849c3c3bfda36788f` |
| `src/app/i18n/messages/en.ts` | `6ae0abd4bbbaff844cee5dbd78da9cbb31ccdb14cdda760ce6b139412c632e47` |
| `src/app/preview/messages/de.ts` | `e9115528589dc3e1904da30c808b3dbbb164fbb79daaf28509812c77a145d2e5` |
| `src/app/preview/messages/en.ts` | `e842d4e3e173f2ef96126def8e8e9bc656d81fa3625f9515cf6fbec98555627e` |
| `src/app/shell/messages/de.ts` | `44a851951c82b6e8436e9757815305a4fb8ee276ca95831902245b096fe7e362` |
| `src/app/shell/messages/en.ts` | `fd1ea0afb7f3373b87d565e9465f180a4244d34f7476aab43eace99b81f3ef33` |
| `src/features/activity/messages/de.ts` | `c9f5297bcba9171fbb364e5f58b606c48e1d7d10e890ce9b2747bae6b0f21df8` |
| `src/features/activity/messages/en.ts` | `927fdfc56b7cef29876aaaa160c148752d4aa7e15fe897988fb5357a6a0478d8` |
| `src/features/auth/login/messages/de.ts` | `fc18b4ff9a7997507dc122d1f7fdfa00766e250cb99a2015d45f27f432129250` |
| `src/features/auth/login/messages/en.ts` | `01020d358fb153c955a27b588806237bd114b384892ed6b0210443600412358f` |
| `src/features/containers/messages/de.ts` | `47fa74a7f265ca7fbe7f7d42ed9513a4dcc2d2e6c6da1ea0a935dbcd4c5c571d` |
| `src/features/containers/messages/en.ts` | `a3f029bdcd63d92737a731e8b575a750e1bbf8c7d6898d79ac048f74c71c7327` |
| `src/features/dashboard/messages/de.ts` | `7689127e3c7df69cc342990e1f0b8685434c934edecaac243c3250e9a6215f64` |
| `src/features/dashboard/messages/en.ts` | `05ad8fc39dff2110cff6a94697cf502366c6885787d3e979c40e2e0157d71190` |
| `src/features/design-lab/messages/de.ts` | `f941181f5098d40bed583469bc7c2729dc6c0891cf46d59a4a6022865b5f206c` |
| `src/features/design-lab/messages/en.ts` | `2f2e8b7d704f1c3e31d50aa5cadfd6a09d9a7cc3cdb9cfd5dca25a2e55b002ee` |
| `src/features/design-lab/messages/visual-de.ts` | `8af269911687ecf5fc9ec4be6dd7ed05ad6fd210a25e018dc7c23a16313d0443` |
| `src/features/design-lab/messages/visual-en.ts` | `0638a00079ff908153731d7d6043c3689b612e3ef71466e0f0569c10c02c5439` |
| `src/features/hosts/messages/de.ts` | `ddd3810607eb5094002af5ba297ec0bc823d4f8dc397e416363215e4599918f3` |
| `src/features/hosts/messages/en.ts` | `1546f89d9444abac690172281508e5a97daa43e7e8387825f21a1860a1396900` |
| `src/features/images/messages/de.ts` | `1d889215efdf9c8103b2108d8bea49dc366922e1b41c4dc6f2998b1d02380030` |
| `src/features/images/messages/en.ts` | `ab00893f1b35a21e49ea630373281bb7f7fc2451c228bab8b426520265920fa0` |
| `src/features/networks/messages/de.ts` | `c29ba24c6834b5406ead815a2e260103b9c91eeaa447cd78c196f4867ab64dce` |
| `src/features/networks/messages/en.ts` | `cce1b58494affc146b4c66b1baea9074182ad49635af51f0263e32ba2c4c735a` |
| `src/features/networks/messages/pool-de.ts` | `341b8bf0d47dac5f472adaad4546e3da0c94d66d0209f0fb22ee6123fa26bf62` |
| `src/features/networks/messages/pool-en.ts` | `88853e7fdaa9ca6dcdf4a588091fc37ded0e5128049cb8305b82f194886476de` |
| `src/features/security/messages/de.ts` | `74c9115bc1eefadb4c0de0c2322d709bdbde87d0dec0cb4318363bdfcbb86dd1` |
| `src/features/security/messages/en.ts` | `c632291389a27a66ed1ce0fe5059ffb40172c39e08c1da64322ebd94218296d9` |
| `src/features/settings/messages/de.ts` | `cb93d80db5b6979f6ae3f3253884563643ea35ba57ac2588a380d7ed63713312` |
| `src/features/settings/messages/en.ts` | `747254e2620528ca922ef71c066f3d796fa7b856268644e2517f16edf5b0f4c5` |
| `src/features/setup/messages/de.ts` | `103773719a6c6d5741632d0ad60a9029a04594efb578f64719f400db4367c565` |
| `src/features/setup/messages/en.ts` | `5ef471e597c5415a9b4d507a040d72c538aa3e3a77ab918a7b3624493551adb6` |
| `src/features/stacks/editor/messages/de.ts` | `697291f92d37180a45c0189ef149032bc03218f738f3864072208ac4773a9e28` |
| `src/features/stacks/editor/messages/en.ts` | `1c83729f37a34b45440c113165e030fa77220dd83559e7b6b272c4f439fe3061` |
| `src/features/stacks/messages/de.ts` | `3b038aae5bc4f534ad39cc5a9a7138ea098bb2eee99dd310652fb1ccdab7d6b7` |
| `src/features/stacks/messages/en.ts` | `53993b7ac3e76ef55fcf06a4a0aef1cb196da33fc2756645762759ac1ee30b4a` |
| `src/features/volumes/messages/de.ts` | `d95f6f1f430165044f853a757b05ee3746dc60e9ee7b05e598b7fbb5abef2163` |
| `src/features/volumes/messages/en.ts` | `8aaa62e51922e86123b04fb3697806d2881f61bc8ed3a59a878833736726de6c` |
| `src/shared/help/messages/de.ts` | `6c4d56088f30bee81bef7790554c0b7d5afda2ea0f17b29669471e25c3e959cc` |
| `src/shared/help/messages/en.ts` | `3c5d2f6231dacb0bc17c9c005fd91c137d4b91ae069d971eaf94623a8e5a31ac` |
| `src/shared/messages/de.ts` | `ea80e44e5382800b49dd825b3d3f9e6707add4aa100a5d39624a986d71e46d45` |
| `src/shared/messages/en.ts` | `ef481cb798f2bfa9200ffbfbfc235d6cdfaebc5547ef10dd8871e36bdc5e252a` |
| `src/shared/theme/messages/de.ts` | `c5527ec5e0ac2c15a1a4533aa6fc385e07a633b28be4cdebcbd5b14d97470379` |
| `src/shared/theme/messages/en.ts` | `cb92c241513045ff1c4a462bb6b363996b2a17ed579550d7c9949e5e9124379e` |
| `src/shared/workspace/messages/de.ts` | `36b5c87153633eb2951721264481f743c430669c6a9111d70125f5cc164240eb` |
| `src/shared/workspace/messages/en.ts` | `31d3be462d416e43fcce8dd7b81e63c9fae4d295dd0cf3e30ea3c95d3f360620` |

English now defines each feature/platform catalogue type, including relocated
preference/visual messages, address-pool and disconnected-backend messages, the aggregate `Messages` type and use-intl's configuration.
German follows those types. The default and invalid-preference fallback remain
German; the locale store, storage key, switching and formatting behaviour stay
unchanged. The original snapshot aggregate remains the historical import record.
Per-file provenance sidecars retain source revisions and identify subsequent
changes; new guard support files have their own first-party content revisions.

Every exported message value was extracted before and after this change by
transpiling the catalogue modules with the installed TypeScript compiler and
walking their exports recursively, including arrays. The ordered JSON map uses
`web/<file>/<export>/<nested/key>` keys from `git ls-files web/src` order.
Its **6,638 values** are byte-identical in UTF-8. SHA-256 of the compact
`JSON.stringify` map is
`cadb095d35302f74ac6d3c428080c3410173b142bb5c43c38ab98c3756b87486`
both before and after. Only imports, type annotations and catalogue comments
changed; no existing DE/EN message was rewritten.

The lab's German file-hash assertion is retired. Tests now validate all 24
current catalogue pairs, including the relocated preference and new backend
pairs, and the merged catalogue: keys/array entries, ICU syntax in both
languages, argument names/types, rich tags, plural/select branch keys, nested
branch arguments, ordinal/cardinal kind, offsets and apostrophes. Mutation tests
cover missing/extra keys, mismatched arguments, malformed syntax and branch drift.
Both rendered catalogues are also formatted with sample arguments.

`@formatjs/icu-messageformat-parser` **3.5.18 (MIT)** is an exact development
dependency so tests use the same parser version already pinned transitively by
`intl-messageformat` in use-intl. No runtime dependency or parser version was
added. Its installed `LICENSE.md` carries the MIT notice. This avoids maintaining
a partial ICU parser. A full distribution dependency inventory remains a separate
M2/release duty as described above.

`openapi-typescript` **7.13.0 (MIT)** is an exact development dependency used only
to generate TypeScript declarations from the local OpenAPI export. It adds no
runtime client. Of its 25 added transitive packages, the non-MIT licences are
ISC (`minimatch`, `yargs-parser`), Apache-2.0 (`yaml-ast-parser`), Python-2.0
(`argparse`) and MIT OR CC0-1.0 (`type-fest`); the other 20 are MIT. Its declared
TypeScript peer range is `^5.x`, while this workspace pins TypeScript 6.0.3.
Generation and typechecking pass with that peer mismatch, which needs checking
again before upgrading either tool. The generated declarations have an exact
ESLint exclusion and remain included in the TypeScript project.

There are **22 existing English messages with undoubled apostrophes** in the
snapshot. The no-text-rewrite boundary preserves them with exact file/key/value
SHA-256 exceptions in `apostrophe-exceptions.test-support.ts`. Any new or modified
violation fails, and unused exceptions fail. The strict validator has no default
exceptions; these apply only to the recorded import. Correcting these message
bytes needs a separately authorized text change.

The language-neutral UI literal guard retains the previous TS/TSX source scope
and documented demo-data exclusions. It checks JSX text and expression output,
accessible/text attributes, text fields and prose independently of German or
English spelling. It skips technical attribute values, translation keys and
style syntax. It is a static guard, not whole-program data-flow analysis; dynamic
backend/user content and arbitrary string indirection still need review.

The **109 exact line-hash exceptions (118 detected occurrences)** distinguish
existing untranslated labels (including imported free primitive labels), technical
notation/input examples, developer diagnostics, paired formatter words and
installer output. No file-wide UI exemption is added. Changed, duplicated or stale
exception lines fail. Existing labels remain an explicit localization follow-up;
new JSX or accessible-attribute literals fail, with injected-source red proofs.

The four imported design-lab catalogue files now live under
`src/shared/preferences/messages/`; the hash table retains their import paths.
The two disconnected-backend catalogues under `src/data/messages/` are new product
texts and follow the same English-base type and parity checks. Comparing the
transpiled export-value map with the pre-change import record after normalizing
the four relocated paths found all 6,638 original values unchanged and exactly
four new backend title/hint values.

## Adapter port and coded messages (#12)

The 592-file snapshot hash above remains the historical import record. Per-file
sidecars identify every file changed after the import. The adapter now returns
run and event codes with parameters; `src/data/message-code.ts` renders them from
the paired catalogues. `src/data/demo/message-codes.ts` maps the imported German
demo phrases to stable names, and `src/data/demo/encode-message.ts` translates
the in-memory fixtures at the adapter boundary. The new catalogue files under
`src/data/messages/` derive their DE/EN wording from the imported demo text
table. `src/data/demo/fixtures/legacy-run-messages.json` freezes that table's
visible output before the port change for byte-for-byte tests.

Demo failure injection now lives in `src/data/demo/options.ts`; the product
interface has no failure parameter or simulation result marker. The imported
`src/shared/hooks/use-demo-text.ts` moved to `src/data/use-adapter-text.ts` so
adapter text is rendered at the data/UI boundary. Its upstream source revision
is retained in the new sidecar; the historical snapshot entry is unchanged.
The production bundle guard also proves that a demo-text import from a shared
hook is rejected.

## Production shell corrections (#11)

The 592-file aggregate remains the historical import record. Current per-file
sidecars record the post-import bytes of the production shell correction. The
host installation sheet was extracted from the imported `HostsPage.tsx` into
`InstallSheet.dev.tsx`. In M14 (#318) the production `InstallSheet.tsx`
replaced that development sheet, and its sidecar entry was removed.
Development-only routes, controls and catalogue values retain their imported
wording in development builds, while the product build uses neutral values or
omits demo-only modules. The disconnected adapter now presents a stable error
for stack creation and rejected source imports. Responsive width changes do not
alter the imported copy. The production bundle guard and route tests provide
the product-boundary evidence.

## Production review follow-up

The historical 592-file import snapshot remains unchanged. Nine updated web files
have current content hashes in their per-file sidecars. The production sidebar
derives its visible entries from the registered route tree. The boundary guard
rejects nested `.dev` modules and the setup, onboarding and preview pages when
imported into a production build. Route tests load the production branches and
check visible navigation against that same tree. The production host list hides
its unavailable onboarding action, and run output omits the development-only
simulation badge.

## HTTP activity and event integration (#38)

The historical 592-file snapshot remains the import proof. Current sidecar
hashes identify the changed React and data files. New HTTP adapter, capability,
DTO mapping and matrix-test files carry their own provenance headers. The
adapter reads the checked-in generated OpenAPI types, retrieves run details and
events, and uses the approved SSE channel to refresh snapshots. Its only new
transport is browser HTTP and EventSource; no runtime package was added.

M12 #284 replaces prototype volume backup plans, history, target settings and their
fixtures with the live v9 backup domain and mappings. New transport and interaction
files are original Apache-2.0 work; changed imported files retain exact sidecar
hashes and deviation records. No dependencies or binary assets were added.
