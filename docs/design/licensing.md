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
Linux/amd64 compiled Go graph and complete Go/web notice bundles; the scratch runtime CA assessment below closes the image-payload boundary for
Linux/amd64. Other targets remain release-assessment boundaries.

M0 established the immutable Arcane licence copy before product-code adoption.
The current [inventory](../provenance/core-import.md) and
[extracted package checks](provenance.md#distribution-hook-points) extend that
foundation; they do not authorise a product release without practical acceptance.

## Scratch hub and agent runtime (Linux/amd64)

The release Dockerfile uses `scratch` for both runtime stages. Its payload is
the static, CGO-free Go binary, the hub's embedded web assets, the existing
Go/web licence bundle, directory metadata and one CA certificate file. No
Alpine executable, libc, shell, package manager or optional GPU tool is copied.
Build-stage utilities are not part of the distributed runtime. This assessment
does not cover the separately maintained tools/helper images.

The copied `/etc/ssl/certs/ca-certificates.crt` comes from Alpine
`ca-certificates-bundle 20260909-r0`, x86_64, in
`alpine:3.22.6@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8`.
Its SHA-256 is
`028fca5150ee1915c4474489985d55c54fceaa680c754cc8d1301734f01b2232`.
The build checks the hash; each image smoke extracts the unmounted file and
compares it with `deploy/release/ca-bundle.json`. A separate mounted-root TLS
smoke proves Go reads the system CA path and refuses an unrelated test CA.

The exact [Alpine packaging source](https://github.com/alpinelinux/aports/tree/19b56f610f4f0055f155e99e1666cddcd000d01c/main/ca-certificates)
declares MPL-2.0 AND MIT for the source package. The copied bundle consists of
Mozilla certificate data generated from MPL-2.0 `certdata.txt`; MIT package
utilities and the source archive's GPL script are excluded. The
[unmodified source form](https://distfiles.alpinelinux.org/distfiles/v3.22/ca-certificates-20260909.tar.bz2)
is identified by SHA-512 in `LICENSES/NOTICE-Mozilla-CA.txt`. Recipients receive
that notice, source URL and the complete `LICENSES/MPL-2.0-Mozilla.txt` in
`/usr/share/licenses/docklet-next/ca/`. The [MPL distribution terms](https://www.mozilla.org/en-US/MPL/2.0/)
permit the larger work while preserving the covered file's licence and source
availability. No Mozilla code or certificate data is modified here.

A new CA hash, runtime payload or platform requires a refreshed assessment and
matching smoke evidence before distribution. The current binary-graph and web
notice requirements still apply independently; this is not product-release or
deployment approval.
