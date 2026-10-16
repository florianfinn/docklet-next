# Licensing and redistribution

Own Docklet Next code is Apache-2.0. Arcane-derived parts retain BSD-3-Clause.
README and NOTICE explain this split; no single unqualified project licence
replaces upstream obligations. Governance tooling is adapted from the maintainer's
Apache-2.0 docklet-hub revision a4484c910bb5d6a8f8df97b4f23f4ee5e6359081.

The examined Arcane licence is from v2.15.1,
`095cb778daba1034e2a39961199e712a75d4ba29`, Copyright (c) 2025, Kyle Mendell.
The required six duties are:

1. Source redistribution preserves copyright, all three conditions and the
   disclaimer verbatim in LICENSES/BSD-3-Clause-Arcane.txt. Keep existing headers
   and add provenance to every adopted source file.
2. Images, agent binaries and installation/release packages include the same
   notice and licence, for example in /usr/share/licenses/ in an image. Provide
   an in-app About / Licences view with third-party texts. M1/M2 packaging and UI
   checks must verify this before any artefact release.
3. Do not use the copyright holder or contributor names for endorsement/promotion
   without permission. Based on Arcane is factual provenance. Product names,
   logos, marketing and palette names must not imply partnership or support.
4. Keep Apache-2.0 for own parts and BSD-3-Clause for Arcane parts with both texts
   and NOTICE. Preserve applicable notices on modified/adopted components.
5. Inventory types/, CLI, all external Arcane modules and all Go/npm dependencies,
   with exact licences and sources. Unknown or incompatible shipped licences
   block release. Re-run the binary graph assessment for the selected M1 revision;
   the examined graph is evidence, not a blanket approval of future versions.
6. CI rejects missing provenance or missing/changed BSD text. M1/M2 add inventory
   drift against actual lockfiles and extracted artefact copies. Independent review
   verifies licences and free-component origins before public release.

The historical pre-cut Go binary graph used go list -deps for backend/cmd and go-licenses
v2.0.1: 297 modules, with 535 reported licence groups (not individual imported
packages). Five reported MPL-2.0 groups require file-level assessment:
filepath-securejoin, errwrap, go-cleanhttp, go-multierror and go-version. Only
BSD files from filepath-securejoin are compiled in the measured graph; its MPL
pathrs-lite tree is excluded. Select BSD-3-Clause (Eclipse Distribution License
1.0, EDL-1.0) for eclipse/paho.golang from its EPL-2.0 OR BSD-3-Clause licence and
say so in NOTICE. All applicable copyright/licence texts travel in the bundle.
The refreshed [adoption inventory](arcane-adoption.md) records 1,587 imported
packages, mixed and nested grants, resolved scanner gaps, and exact-file libc
header exceptions assessed for the pre-cut inventory. The
[SQLite driver decision](sqlite-database.md#driver-provenance-and-licences) requires
a fresh compiled-graph and notice assessment before distribution; the former
removal gate does not approve restored dependencies. The historical assessments exclude the web runtime and image contents. The
[current SQLite assessment](sqlite-license-assessment.md) records the selected
Linux/amd64 compiled Go graph and complete Go/web notice bundles; image OS
packages and other targets remain release-assessment boundaries.

M0 established the immutable Arcane licence copy before product-code adoption.
The current [inventory](../provenance/core-import.md) and
[extracted package checks](provenance.md#distribution-hook-points) extend that
foundation; they do not authorise a product release without practical acceptance.
