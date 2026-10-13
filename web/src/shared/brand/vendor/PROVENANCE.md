# Vendored logo modules

Unchanged copies of the pure renderer and motion modules of the Docklet Next
logo study (`logo-next/src/`, first-party, intended for Apache-2.0 like the
rest of Docklet). Do not edit them here: change `logo-next/src/`, run
`node scripts/sync-react.mjs` there and commit both sides. The test
`brand.test.tsx` fails when a copy differs from its source.

Retrieved: 2026-10-11. The study is not a Git repository, so each file is
identified by its SHA-256.

Not copied: `docklet-logo.js` (DOM module and custom element; replaced by
`../play-entrance.ts`) and `docklet-logo.css` (colour hooks; ported to
`../brand-logo.css` with the app's palette tokens).

| File | Source | SHA-256 |
| --- | --- | --- |
| `candidates-first.js` | `logo-next/src/candidates-first.js` | `126af16c2c6d85d26721e3ade4ba9a0de9f075c30d2d73a5d6ee13802d5bb167` |
| `candidates-second.js` | `logo-next/src/candidates-second.js` | `db99280e7e1b38c006d7e8ba9b857cdf821706db22740d8bcbc43a262b78a7a3` |
| `candidates.js` | `logo-next/src/candidates.js` | `5f0150f31531090889f4c2298a3f7ea867db6eddb7f15d851738bbf0d275d1c6` |
| `construction.js` | `logo-next/src/construction.js` | `0afe45998f8426e1253a6f9be4accf8be56e2e53d37d04e493c6ffd38b74b231` |
| `glyphs.js` | `logo-next/src/glyphs.js` | `13b57d642dc6e168ba7b8998bd5f541ad5563ec494fa67334460133562fce7e6` |
| `lettering.js` | `logo-next/src/lettering.js` | `aa3dbd6ef44190384dbc4880fb40ab9084307816835bcc4bb56c1af3fa44b968` |
| `logo-motion.js` | `logo-next/src/logo-motion.js` | `2961ca7be752e302709ab418a62ab8b1cb77a88ad108b8a1e7ba9a536180bc5a` |
| `logo-svg.js` | `logo-next/src/logo-svg.js` | `e61c2fbc142e826a0652db7ec12bd0d920585a26b15acaaf36f60c4920ad404d` |
| `motion-blueprint.js` | `logo-next/src/motion-blueprint.js` | `67f24e0fed7d9d64d5acaec3271799c61e2bc9bedd189e2dab8c653e1c691ad1` |
| `motion-shared.js` | `logo-next/src/motion-shared.js` | `d08427f9f4bab3f02afe2868aec69d7a4d7781d4b8c82ad2cb7b757a9c62a2df` |
| `motion-stack.js` | `logo-next/src/motion-stack.js` | `4c0e8ccaafbc90641c90ec8af15db9cadad56ac5b2434ff0f26915288e74c923` |
| `shapes.js` | `logo-next/src/shapes.js` | `d59670bc2b0f1a4cc2146bdaa53435badfae0ca99a1a4aa966024ccdc96ba06b` |
