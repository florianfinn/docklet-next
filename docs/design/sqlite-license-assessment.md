# SQLite dependency and notice assessment

The Linux/amd64 hub and agent use libtnb/sqlite v1.2.2 under MIT and
modernc.org/sqlite v1.60.1 under BSD-3-Clause, with SQLite's public-domain
dedication and the driver's aggregated third-party notices. Francis remains MIT
with applicable Apache-2.0 Dapr notices. Both command graphs use CGO disabled.
The current graph and hash-bound legal files are in
[core-import.json](../provenance/core-import.json); the original Arcane evidence
remains historical input rather than blanket approval of new dependencies.

The actual compiled graph contains two declaration-only generated files from
modernc.org/libc v1.77.1. AST inspection rejects executable functions in a file
carrying a copyleft notice; exact paths and whole-file hashes are separately
pinned by the compiled-notice guard. The files are:

- `sys/types/types_linux_amd64.go`, SHA-256
  `1ada8b2599b13281b8fe191cfb24c02896ec3781b95a3b5cf76e1d74424eea2e`.
- `uuid/uuid/uuid_linux_amd64.go`, SHA-256
  `c677ffac9af69102fabc82443b8799b4071c7ead9f02237bb2f01af7782af428`.

Their material consists of generated numerical constants and C type/data-layout
declarations. [LGPL-2.1 section 5](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html)
allows the corresponding object code for this restricted header material without
restriction. The UUID declarations also preserve the explicit
[GCC Runtime Library Exception 3.1](https://www.gnu.org/licenses/gcc-exception-3.1.html)
for the adopted basic type definitions. No GCC implementation is linked. Preserve
the full source notices and corresponding-source references. This assessment
covers these exact bytes and target, with changed or additional material failing
the guard; it does not approve arbitrary LGPL/GPL implementation code.

The runtime dependency licence bundle includes the selected root and nested
legal texts, relevant source copyright evidence, Go's licence, project notices,
and both exact compiled header files. The core bundle records source references
for MPL-covered files. The elected paho.golang alternative remains BSD-3-Clause
(EDL-1.0); corresponding covered MPL sources remain available from the pinned
public references. Unknown modules, changed legal bytes and unassessed compiled
notices fail generation or verification.

The web bundle conservatively covers declared runtime dependencies and their
transitive dependencies, including vendored D3 notices and font OFL texts.
Three published npm archives omit their root licence texts. The package retains
pinned upstream project texts for @schummar/icu-type-parser and victory-vendor;
react-remove-scroll-bar's MIT declaration is paired with a pinned upstream
project licence because its npm gitHead is unavailable publicly. Provenance and
licence guards bind all three texts. The package smoke extracts every bundle and
compares each recorded hash. Container OS package licences and other architecture
or optional-build graphs remain separate release assessments.
