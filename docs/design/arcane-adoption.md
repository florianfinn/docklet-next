# Arcane adoption revision and module boundaries

Adopt Arcane **v2.15.1**, source commit
`095cb778daba1034e2a39961199e712a75d4ba29`, once. Keep backend, CLI and wire types
locally, together with the eleven Arcane helper modules. Use pinned module
references for independent third-party libraries. This selects the input to the
functional cut; it neither imports Go code nor approves a release artefact.

## Revision evidence

On 2026-10-13, the supplied clean Arcane checkout was inspected independently of
the SHA in the earlier design document:

```sh
git rev-parse v2.15.1
git rev-parse 'v2.15.1^{commit}'
git rev-parse HEAD
git status --porcelain --untracked-files=no
```

The annotated tag object is `ffc547f212d674f1761228950ed9a76fe0718577`.
Both the peeled tag and HEAD are
`095cb778daba1034e2a39961199e712a75d4ba29`; tracked files are unchanged.
The generator repeats these checks and hashes the three manifests, their sums,
`go.work` and `go.work.sum`. The design reference happens to match the selected
commit; that equality was verified, not presumed. The tag-object SHA identifies
the annotation, not the source tree. Selection does not depend on a moving
latest-release pointer or establish a cryptographically verified tag signature.

Primary references: [release](https://github.com/getarcaneapp/arcane/releases/tag/v2.15.1),
[workspace](https://github.com/getarcaneapp/arcane/blob/095cb778daba1034e2a39961199e712a75d4ba29/go.work)
and [backend manifest](https://github.com/getarcaneapp/arcane/blob/095cb778daba1034e2a39961199e712a75d4ba29/backend/go.mod).

## Local layout and identities

Use the repository namespace `github.com/florianfinn/docklet-next`. Preserve three
module boundaries with identities `github.com/florianfinn/docklet-next/backend`,
`github.com/florianfinn/docklet-next/types` and
`github.com/florianfinn/docklet-next/cli`. This new project starts without an
inherited `/v2` suffix. The root `go.work` includes those three modules; it is not
itself a module. The backend imports the local types and the retained CLI helpers.
Every consuming `go.mod` also has relative replacements so `GOWORK=off` works.
The import step owns the mechanical module/import/linker-symbol rewrite and its
provenance deviations; these identities are a decision, not existing Go files.

| Source module | Local directory | Reason |
| --- | --- | --- |
| `github.com/getarcaneapp/arcane/backend/v2` | `backend/` | Shared hub/agent operating core, maintained independently |
| `github.com/getarcaneapp/arcane/types/v2` | `types/` | Shared wire DTOs, not a remote version that can drift from the core |
| `github.com/getarcaneapp/arcane/cli/v2` | `cli/` | Retained under R5; Edge TLS imports `cli/v2/pkg/generate` |

All three sources are subdirectories of the pinned Arcane commit and retain its
BSD-3-Clause notice, Copyright (c) 2025, Kyle Mendell. The upstream manifests
require CLI/types `v2.14.0`, but local replacements and workspace selection use
the v2.15.1 checkout. Both the declared and effective versions appear in the map.
The upstream `github.com/docker/buildx` replacement selects `v0.32.1`; preserve
that resolution until a separately tested dependency change, not the lower
require version alone.

Each helper below is copied once to `third_party/arcane/<path>`, preserving its
original module identity, exact module contents and existing legal notices.
Keep `go.mod` boundaries and add explicit relative replacements in every
consuming module, including helper-to-helper edges. Do not rely on only the root
workspace. Never overlay all modules from a single Kit HEAD: nested Docker and
sys modules below intentionally originate from different commits. Their module
ZIPs exclude nested modules, so adopt each listed module separately.

| Module suffix after `go.getarcane.app/` | Local path below `third_party/arcane/` | Version | Kit source commit | Reason |
| --- | --- | --- | --- | --- |
| acfs | acfs | v0.6.2 | 3da7cfd43c4da62bc06fd384b7da5f109ce010fb | Project/volume files and Edge certificates |
| builds | builds | v0.4.4 | 83d91f19f60b2967cc85c46161c98203e80e4989 | Compose builds remain supported |
| docker | docker | v0.1.0 | e3d12f637964a6edd495f9b31a10d2f0a40fa127 | Host/container/image operations |
| docker/compat | docker/compat | v0.1.2 | de96f232f840ee55de637f1178f8cb890de9a642 | Docker API and Compose compatibility |
| docker/convert | docker/convert | v0.3.4 | ad1f8d7e16f4bfaf4f6dae6b35d9ea1e3ef961b5 | One-time Docker-run import |
| kit | kit | v0.4.0 | 9c61043fc52d1977d644f075d9c168335c3a6df7 | Shared utility and DTO mapping functions |
| streams | streams | v0.4.5 | 2786ddf03ec35e01655d19b271591436d0057f79 | Activity, statistics and events |
| sys/cgroup | sys/cgroup | v0.2.4 | cefb3a12f0e22cbd2123d51cf397508db9aedcf8 | Host and container resource metrics |
| sys/crypto | sys/crypto | v0.3.0 | e2a909ce45597a3c101dca4d218baef4f89bb643 | Secrets and Edge TLS |
| updater | updater | v0.11.5 | 119f68956cee6d5aba8a5463561aaf9be9943b65 | Image update detection and installation |
| sys/bytes | sys/bytes | v0.3.0 | 60f275d8746e5aff375a298b6fef1c0a2f373296 | CLI size formatting |

All eleven resolve to [getarcaneapp/kit](https://github.com/getarcaneapp/kit)
through the downloaded module's Go proxy origin metadata, with BSD-3-Clause
licences. The map records exact source subdirectories, full commits, Go sums,
licence paths and content hashes. `kit` is the repository root; other suffixes
are source subdirectories. For `sys/*`, retain both the ZIP's LICENSE (Arcane,
2026) and the source repository's `sys/LICENSE` (Kyle Mendell, 2025). Both have
been fetched and hashed. Other copyrights differ by module and are not replaced
with the Arcane backend copyright.

Local adoption supports the agreed independent maintenance and removal of
unwanted functions without depending on the vanity domain. A generated `vendor/`
tree is unsuitable as the authoritative editable fork. Other libraries remain
exactly pinned remote Go modules with sums; an eventual offline vendor bundle is
a packaging output. No missing source requires a substitute: all 334 exact
module/version records were source-resolved. `unsourceableModules` and
`blockedModules` are empty, rather than silently omitting missing modules.

## Inventories and reproducibility

The [human module table](../provenance/arcane-modules.md) and
[machine map](../provenance/arcane-modules.json) cover **419 require entries** from
backend/cli/types, represented by **334 exact module/version records**, including
selected binary modules and workspace roots. Separate declared versions remain
visible when workspace selection or replacement changes the effective version.
No `go list -m all` entry is automatically called a binary dependency.

The [binary inventory](../provenance/arcane-binary.json) contains **297 modules**.
The four `arcane-package-graph-*.json` shards record **1,587 non-standard-library
packages**, with hub/agent membership and licence-group references. The
[licence-group inventory](../provenance/arcane-license-packages.json) preserves the
535 groups returned by go-licenses, their resolutions and exact legal-file hashes.
A group may cover multiple imported packages. Module rows carry copyright,
modifications, source, redistribution obligations and accepted/blocked disposition;
package and group records join to those rows by module name and exact version.

The upstream hub and agent share `./backend/cmd/...`. The hub measurement uses
linux/amd64, CGO enabled and no tags; the agent measurement uses linux/amd64,
CGO disabled and `exclude_frontend`. This tag omits frontend embedding, not
SQLite. The inventory is before the functional cut. Other platforms, optional
`buildables`/`playwright` tags, the standalone CLI's complete binary graph,
web dependencies and container contents need separate release inventories.
Non-binary declared entries conservatively have build/test scope; the complete
937-module workspace resolution is not an exhaustive test/build licence inventory.
The Go standard library has its own version and hashed BSD-3-Clause notice in
the inventory metadata and is not included in these module counts.

With Node 24, Go 1.27.2, Git, network/module-cache access and the pinned clean
checkout, run from Arcane's root (set the three variables to local locations):

```sh
export GOOS=linux GOARCH=amd64 CGO_ENABLED=1 GOFLAGS=
go run github.com/google/go-licenses/v2@v2.0.1 report ./backend/cmd/... > "$LICENSE_REPORT"
node "$DOCKLET_REPO/scripts/inventory-arcane.mjs" "$ARCANE_CHECKOUT" "$LICENSE_REPORT" "$DOCKLET_REPO/docs/provenance"
```

The generator uses only Node built-ins; go-licenses is a pinned external
measurement tool, not a new product or Node dependency. It reads sources and
writes only the selected output directory. It verifies the source revision,
downloads the exact declared modules, independently lists both binary graphs,
checks licence-group coverage of every imported package, and reads/hash-checks
licence evidence. Legal-file paths are relative to their cited module ZIP,
or to the Arcane repository root for sibling modules; supplemental repository
files have direct immutable URLs. Module source URLs without proxy origin metadata point to the
exact versioned module ZIP, verified by its Go checksum. Older proxy records need
not expose a full VCS SHA to remain retrievable and precisely identified.
JSON uses one package record per line; graph shards contain at most 500 records.
No size-guard exemption is used. The measurement date is deliberately fixed to
this adoption decision; changing the revision/date requires a fresh review.

## Manual licence resolutions and distribution duties

All 214 scanner `Unknown` entries have source evidence, not an assumption based
on a repository name. The module inventory stores the reviewed licence text hash:

| Scanner gap | Resolution and evidence |
| --- | --- |
| Arcane backend, CLI and types | Root `LICENSE` outside the module directories; the exact BSD text already lives in `LICENSES/BSD-3-Clause-Arcane.txt` |
| Ten helper modules in the hub graph | Each module ZIP's `LICENSE`; BSD-3-Clause. `sys/bytes` is additionally mapped from CLI declarations |
| `github.com/in-toto/attestation` and `github.com/in-toto/in-toto-golang` packages | Their pinned root `LICENSE` files explicitly grant Apache-2.0 |
| `gotest.tools/v3` packages | Root Apache-2.0 `LICENSE`; retain the separately detected BSD-3-Clause `internal/difflib/LICENSE` |
| `github.com/mattn/go-localereader` v0.0.1 | `README.md` explicitly says MIT and credits Yasuhiro Matsumoto. No separate copyright statement or licence file is supplied; preserve that declaration and credit, without inventing a notice |

Root licence scanning also corrects incomplete single-licence group labels for
mixed-licence modules, including both YAML v3 module paths. Both Apache-2.0 and
MIT apply to different files there; this is not a choice between alternatives.
Nested licences remain separate evidence even if they match the root SPDX name.
Copyright excerpts omit email addresses; these inventory excerpts are not a
replacement for verbatim source notices or a release licence bundle.

Five go-licenses groups identify MPL-2.0: filepath-securejoin v0.7.0, errwrap
v1.1.0, go-cleanhttp v0.5.2, go-multierror v1.1.1 and go-version v1.9.0.
`filepath-securejoin/COPYING.md` expressly states BSD-3-Clause AND MPL-2.0;
file headers determine applicability. Preserve covered-file notices, distribute
the MPL text and make the corresponding covered source, including any changes,
available to binary recipients. Do not relicense those files as Apache-2.0.
Nested buildx licences and modernc's third-party notice aggregations also remain
recorded; an aggregate mentioning MPL does not prove another compiled MPL group.

For eclipse/paho.golang v0.23.0 select **EDL-1.0** from EPL-2.0 OR EDL-1.0;
its `LICENSE` explains dual licensing and `edl-v10` supplies the selected text.
The exact source reference and hashes are in the inventory and NOTICE.
All other applicable licence/copyright/NOTICE texts must also travel with release
artefacts. SQLite's public-domain statement (`LicenseRef-SQLite-Public-Domain`, defined
by the hashed `LICENSE-SQLITE`) and bundled SQLite-vector MIT text,
and libc's embedded third-party notices, must not be lost by looking only at a
module's primary SPDX identifier. These source assessments allow the adoption;
post-cut lockfile drift, licence-bundle construction and extracted-artefact checks
remain the import/packaging owners' release gates. No product binary ships here.

## PostgreSQL-only boundary

The [SQLite removal map](arcane-sqlite-removal.md) enumerates the source paths,
90 migration files, tests, configuration and build surfaces for #7. Remove SQLite
provider support, including Francis sidecar storage, rather than only the driver
import. Preserve database-independent behaviour tests through pure logic or
appropriate PostgreSQL-specific checks without requiring a running service in
the default test chain. System backup/recovery is SQLite-specific upstream;
#7 must remove that implementation and state the unavailable capability until a
PostgreSQL implementation exists. This does not silently remove the retained
backup feature. Existing Arcane database migration or recovery is not supported.
