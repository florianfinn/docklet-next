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
been verified and hashed; the supplementary text equals the committed Arcane BSD
text and is derived from it during offline regeneration. Other copyrights differ by module and are not replaced
with the Arcane backend copyright.

Local adoption supports the agreed independent maintenance and removal of
unwanted functions without depending on the vanity domain. A generated `vendor/`
tree is unsuitable as the authoritative editable fork. Other libraries remain
exactly pinned remote Go modules with sums; an eventual offline vendor bundle is
a packaging output. No missing source requires a substitute: all 334 exact
module/version records were source-resolved. `unsourceableModules` and
`blockedModules` are empty, rather than silently omitting missing modules.

## Verbatim import tree evidence for #6

Record a verbatim import commit first and compare its directory tree hashes,
then perform module/import-path rewrites and provenance-header additions in a
separate mechanical commit. Hash comparisons apply to the unmodified import;
rewritten trees cannot equal their upstream input. The machine map carries each
`sourceTree` plus source revision and module sums.

| Import input | Git tree SHA-1 |
| --- | --- |
| `github.com/getarcaneapp/arcane/backend/v2` | `db671029c48605e97646ba82bab6e9e4f92e7575` |
| `github.com/getarcaneapp/arcane/cli/v2` | `b4fa01d5d47d091777e8014e42705e3a89c98cf7` |
| `github.com/getarcaneapp/arcane/types/v2` | `67aef30853bee0590226137d922c5716c848c233` |
| `go.getarcane.app/acfs` | `72150ec8c6d74991db6421a4030146d9d7629bad` |
| `go.getarcane.app/builds` | `45160bd3f0c1cf11796d73e298838f10b0a3ed87` |
| `go.getarcane.app/docker` | `dd1cd368e9a41b85c3a154d16b1e2e62e0cc4671` |
| `go.getarcane.app/docker/compat` | `e76be4061fbb594e7a98b495192e5058959ec8cc` |
| `go.getarcane.app/docker/convert` | `4f05acf5068d63c42b7d088acf51304bc88594ee` |
| `go.getarcane.app/kit` | `b8ce4c929d1ef53df74495bd0a9e7281efe3cdec` |
| `go.getarcane.app/streams` | `2106ec978d9f7bc3cba2aea7cc17bcdfce1e1050` |
| `go.getarcane.app/sys/bytes` | `8eb616b8403c771ea7a74025155c2c0b66a88ae7` |
| `go.getarcane.app/sys/cgroup` | `dcfc4b43513b33c3bab20979b59dcb5c64f8c0f2` |
| `go.getarcane.app/sys/crypto` | `79e90751a3b18a63b032a59e510947814b02da57` |
| `go.getarcane.app/updater` | `fef1384f5c64e534b1b2fdcf06c31bf947b44048` |

The three Arcane hashes are upstream Git subtrees (`git rev-parse SHA:path`).
Helper hashes are Git trees of the exact extracted Go module ZIP contents, with
all regular files normalized to mode `100644`, as supplied by Go; directories
use `40000`. They are deliberately not repository subtree hashes: Go ZIPs exclude
nested modules and other repository-only files. Compare each helper separately
before overlaying its separately versioned nested helpers. The generator hashes
Git blob/tree object encodings directly, without writing into source checkouts.
For a validation with Git, stage the extracted module in an empty repository
with `core.autocrlf=false`, normalize file modes, then run `git write-tree`.

#6 must also add every helper licence variant to the immutable licence hash
manifest. Use the full `licenseFiles[].sha256` values, including the four distinct
root LICENSE variants and the supplemental `sys/LICENSE`; do not substitute one
module's copyright or licence bytes for another's.

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
The 24 non-binary entries declared by CLI carry `runtime (cli)` scope so their
runtime duties are included pending a standalone CLI measurement. This also
conservatively includes declared test dependencies and a superseded types version.
Other non-binary entries carry build/test scope; the complete 937-module workspace
resolution is not an exhaustive test/build licence inventory.
The Go standard library has its own version and hashed BSD-3-Clause notice in
the inventory metadata and is not included in these module counts.

With Node 24.21.0, Go 1.27.2, Git, the pinned clean checkout and the already
populated Go module cache, run from this repository:

```sh
node scripts/inventory-arcane.mjs "$ARCANE_CHECKOUT" \
  docs/provenance/arcane-scanner-input.json docs/provenance
```

The committed scanner input contains the 535 package/licence pairs from
`go-licenses/v2@v2.0.1 report ./backend/cmd/...`; its network-resolved URL column
is deliberately discarded. Its SHA-256 is `tool.reportSha256`. The generator
constructs evidence references from immutable source commits or versioned module
ZIPs with Go checksums and exact file hashes. It never resolves scanner URLs.
The three supplementary Kit `sys/LICENSE` files equal the pinned Arcane root
licence; the generator verifies its recorded hash and derives those bytes locally.
`arcane-license-reviews.json` records exact module/version/path/hash decisions,
including all previously unclassified evidence and excluded source filenames.
Changed exception bytes or new unclassified legal files fail regeneration.

Go and Node must see identical absolute paths, including `go env GOMODCACHE`
and `GOROOT`. A host Node process cannot read paths returned by a container-only
Go wrapper. With containerized Go, place a compatible Node 24.21.0 binary in the
shared workspace and run both tools in the same container. Set `WORKSPACES`,
`DOCKLET_REPO`, `ARCANE_CHECKOUT` and `GO_CACHE_VOLUME` to existing local locations
and the prepopulated cache volume. No port is exposed:

```sh
mkdir -p "$DOCKLET_REPO/.scratch"
cp "$(command -v node)" "$DOCKLET_REPO/.scratch/node"
docker run --rm --network none --label t3.project=docklet-next \
  --user "$(id -u):$(id -g)" \
  -e GOTOOLCHAIN=local -e GOPROXY=off -e GOSUMDB=off \
  -e GOMODCACHE=/go-cache/mod -e GOCACHE=/go-cache/build \
  -v "$GO_CACHE_VOLUME:/go-cache" \
  -v "$WORKSPACES:$WORKSPACES:ro" \
  -v "$DOCKLET_REPO:$DOCKLET_REPO" \
  -v "$DOCKLET_REPO/.scratch/node:/usr/local/bin/node:ro" \
  -w "$DOCKLET_REPO" \
  golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 \
  node scripts/inventory-arcane.mjs "$ARCANE_CHECKOUT" \
  docs/provenance/arcane-scanner-input.json .scratch/reproduced
for file in .scratch/reproduced/*; do
  cmp "$file" "docs/provenance/${file##*/}" || exit 1
done
```

Populate the module cache before going offline: it must contain all exact module
versions, ZIP metadata and sums needed by the three manifests and both graphs.
The generator forces `GOPROXY=off`, `GOSUMDB=off` and the local toolchain; missing
cache entries fail rather than fetch. Cached proxy origin metadata identifies
source commits; older records without origin metadata use checksum-pinned ZIPs.
No absolute cache or checkout path enters generated output. A clean output-dir
rerun on 2026-10-13 with networking disabled reproduced all eight files byte for
byte. This proves reproducibility from the committed scanner pairs, review policy,
pinned checkout and cached modules, not reproducibility of go-licenses' URL lookup.

The generator uses only Node built-ins. It verifies the source revision, reads
exact cached modules, lists both command graphs, checks every imported package's
licence-group coverage and hashes legal evidence. Legal-file discovery excludes
source/workflow filenames and includes REUSE `LICENSES/` directories. It collects
all grants in combined and nested legal files, separating explicitly reviewed
asset/documentation terms from code expressions. It also checks compiled Go/C/H/S
comments for copyleft/source-available markers. MPL/EPL references to secondary
licences and the libc headers have exact hash-bound reasons; no generic module
exemption exists. JSON uses one package record per line; graph shards contain at
most 500 records. No size exemption is used. The date is fixed to this adoption
measurement; a new revision/date requires review.

## Manual licence resolutions and distribution duties

All 214 scanner `Unknown` entries have source evidence, not an assumption based
on a repository name. The module inventory stores the reviewed licence text hash:

| Scanner gap | Resolution and evidence |
| --- | --- |
| Arcane backend, CLI and types | Root `LICENSE` outside the module directories; the exact BSD text already lives in `LICENSES/BSD-3-Clause-Arcane.txt` |
| Ten helper modules in the hub graph | Each module ZIP's `LICENSE`; BSD-3-Clause. `sys/bytes` is additionally mapped from CLI declarations |
| `github.com/in-toto/attestation` and `github.com/in-toto/in-toto-golang` packages | Their pinned root `LICENSE` files explicitly grant Apache-2.0 |
| `modernc.org/libc` v1.77.1 | Root BSD-3-Clause plus MIT in `LICENSE-3RD-PARTY.md`, detected without a module-specific licence override; two compiled header notices and their narrow rationale are recorded below |
| `gotest.tools/v3` packages | Root Apache-2.0 `LICENSE`; retain the separately detected BSD-3-Clause `internal/difflib/LICENSE` |
| `github.com/mattn/go-localereader` v0.0.1 | `README.md` explicitly says MIT and credits Yasuhiro Matsumoto. No separate copyright statement or licence file is supplied; preserve that declaration and credit, without inventing a notice |

Combined root and nested licence scanning corrects incomplete single-licence
labels, including klauspost/compress, gods, pgerrcode, OpenTelemetry and both
YAML v3 module paths. Both Apache-2.0 and MIT apply to different YAML files; this is not a choice between alternatives.
Nested licences remain separate evidence even if they match the root SPDX name.
Copyright excerpts omit email addresses; these inventory excerpts are not a
replacement for verbatim source notices or a release licence bundle.

Five go-licenses groups identify MPL-2.0: filepath-securejoin v0.7.0, errwrap
v1.1.0, go-cleanhttp v0.5.2, go-multierror v1.1.1 and go-version v1.9.0.
`filepath-securejoin/COPYING.md` expressly states BSD-3-Clause AND MPL-2.0;
file headers determine applicability. Only the BSD-3-Clause root and
`internal/consts` packages are compiled here; MPL `pathrs-lite` is not. The module
expression remains conservative. Preserve covered-file notices, distribute
the MPL text and make the corresponding covered source, including any changes,
available to binary recipients. Do not relicense those files as Apache-2.0.
Nested buildx licences and modernc's third-party notice aggregations also remain
recorded; an aggregate mentioning MPL does not prove another compiled MPL group.

For eclipse/paho.golang v0.23.0 select **BSD-3-Clause (EDL-1.0)** from
`EPL-2.0 OR BSD-3-Clause`. EDL-1.0 is the Eclipse name, not an SPDX identifier;
its `LICENSE` explains dual licensing and `edl-v10` supplies the selected text.
Retain both the Eclipse Foundation template attribution and the compiled source
notice for Contributors to the Eclipse Foundation. The exact source reference
and hashes are in the inventory and NOTICE.
All other applicable licence/copyright/NOTICE texts must also travel with release
artefacts. SQLite's public-domain statement (`LicenseRef-SQLite-Public-Domain`, defined
by the hashed `LICENSE-SQLITE`) and bundled SQLite-vector MIT text,
and libc's embedded third-party notices, must not be lost by looking only at a
module's primary SPDX identifier. These source assessments allow the adoption;
post-cut lockfile drift, licence-bundle construction and extracted-artefact checks
remain the import/packaging owners' release gates. No product binary ships here.

## Compiled libc header notices

`modernc.org/libc` v1.77.1 contains LGPL-2.1-or-later notices from glibc in
`sys/types/types_linux_amd64.go` and `uuid/uuid/uuid_linux_amd64.go`. Both files
are compiled in hub and agent; their hashes, copyright notices and reasons are
stored as `compiledLicenseNotices`. The reviewed material consists of
header-derived type layouts and constants. LGPL-2.1 §5 permits object code using
only numerical parameters, data-structure layouts/accessors and small macros or
inline functions from headers without restriction. Comments are not object code;
retain source notices. This is an exact-file assessment, not approval of arbitrary
LGPL code or of adopting libc locally.

The UUID file also preserves GCC's `stddef.h` notice: GPL-3.0-or-later with the
explicit GCC Runtime Library Exception 3.1. Record that notice alongside LGPL;
the basic C type definitions are not a linked GCC implementation. This exact-file
assessment covers the pre-cut inventory, not a future product graph. The original
removal check traversed both dependency graphs with SQLite excluded and required
libc to be absent. Under the [SQLite decision](sqlite-database.md), implementation
step 2 replaces that boundary with an assessment of the actual compiled files,
exact hashes, selected grants and packaged notices for both binaries. Changed or
new material needs fresh assessment before release.

## Database adoption boundary

The [SQLite database contract](sqlite-database.md) supersedes the original
PostgreSQL-only choice. Recover only its named driver/file handling and the
matching Francis SQLite component with provenance. Keep the fresh retained-model
schema, no old-data import and default tests without real services. The
[historical removal map](../history/m1-sqlite-removal.md) records all upstream
paths and the original #7 verification scope. System database backup/recovery
remains owned separately by #32.
