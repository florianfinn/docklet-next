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

The examined Go binary graph used go list -deps for backend/cmd and go-licenses
v2.0.1: 297 modules, with 535 reported packages. Five MPL-2.0 packages require
file-level notices and source references: filepath-securejoin, errwrap,
go-cleanhttp, go-multierror and go-version. Select EDL-1.0 for eclipse/paho.golang
from its dual EPL-2.0/EDL-1.0 licence and say so in NOTICE. All other package
copyright/licence texts also travel in the licence bundle. The prior assessment
excluded the web runtime and image contents; M2 and release acceptance cover them.

M0 copies the Arcane licence byte-for-byte and checks its recorded SHA-256. It
contains no Arcane product code, executable product or completed dependency
inventory. The explicit future checks are specified in [provenance](provenance.md).
