<!--
Source: approved Docklet design prototype (maintainer's own work)/react-opus
Revision: d75474089b87ab9f77c606cc7aa696d05157d8e9cd902f1ed5a0f2b904722a39
Retrieved: 2026-10-13
License: Apache-2.0 AND MIT AND BSD-3-Clause
Deviations: Historical import proof with subsequent product-boundary changes documented below.
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

At the initial import, `web/src/` contained **592 files**. Its SHA-256 aggregate was
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

The only file changed during the initial import was `package.json`: its name is `docklet-next-web`
instead of the prototype package name. All dependency versions, scripts and
other package fields are unchanged. The lockfile and workspace settings are
byte-identical, including the reviewed esbuild build permission.

`web/` remains a self-contained pnpm package/workspace with its own lockfile.
Use `pnpm --dir web install --frozen-lockfile`. Existing root lint, test and build
commands already delegate to this package; existing change-scope rules already
classify all `web/` changes as executable. No guard script changes are needed.
CI enables pinned pnpm 11.21.0 and installs the web lockfile before running root
lint (ESLint plus TypeScript), tests and build. The existing SHA-pinned actions,
read-only permissions, text-only fast path and 15-minute timeout are retained.

No dependencies, build output, lab documentation tree, design book, environment
files, screenshots, videos, renders or prototype README are copied. This new
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

This is the approved prototype snapshot. Demo transport and design-lab controls
remain present intentionally; their production separation, locale base changes,
HTTP/WebSocket integration and live product acceptance belong to later steps.

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
