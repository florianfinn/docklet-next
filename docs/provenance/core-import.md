# Core import evidence, 2026-10-13

The import uses Arcane v2.15.1 at
`095cb778daba1034e2a39961199e712a75d4ba29` and the eleven independently pinned
helper modules listed in [the adoption decision](../design/arcane-adoption.md).
The pristine tree proofs below describe the original adoption. The current
SQLite source selection is recorded separately in core-import.json and in the
2026-10-16 assessment below.

## Pristine tree proof

Publication and provenance guards reject the pristine sources: headers and exact
publication exceptions are missing, and a CLI filename triggers the path guard.
The import therefore combines pristine-copy and mechanical adaptation into one
commit. No hook is bypassed. Each tree below was verified before adaptation,
using a separate temporary index initialized with `git read-tree --empty`,
`git -c core.autocrlf=false add -f -- "$path"`, `git write-tree` and
`git rev-parse "$tree:$path"`. Each helper was verified before adding any nested
helper; otherwise the parent Docker tree would include separately pinned modules.

For independent reproduction, export the three directories with
`git -C "$ARCANE_CHECKOUT" archive "$REV" backend cli types` into an empty
scratch repository. The export excludes ignored build outputs, including the
local upstream frontend placeholder. Extract each helper from the exact Go
module cache version recorded in `arcane-modules.json`, normalize regular files
to mode `100644`, and hash each in its own empty index before overlaying nested
modules. Source checkouts are read only. The source helper tree algorithm and
all expected hashes are also in `scripts/inventory-arcane.mjs`.

```sh
proof_dir=$(mktemp -d)
export GIT_INDEX_FILE="$proof_dir/index"
git read-tree --empty
git -c core.autocrlf=false add -f -- "$path"
tree=$(git write-tree)
git rev-parse "$tree:$path"
unset GIT_INDEX_FILE
rm -r "$proof_dir"
```

| Input directory | Verified tree | Files |
| --- | --- | ---: |
| `backend` | `db671029c48605e97646ba82bab6e9e4f92e7575` | 952 |
| `cli` | `b4fa01d5d47d091777e8014e42705e3a89c98cf7` | 90 |
| `types` | `67aef30853bee0590226137d922c5716c848c233` | 99 |
| `third_party/arcane/kit` | `b8ce4c929d1ef53df74495bd0a9e7281efe3cdec` | 55 |
| `third_party/arcane/acfs` | `72150ec8c6d74991db6421a4030146d9d7629bad` | 36 |
| `third_party/arcane/builds` | `45160bd3f0c1cf11796d73e298838f10b0a3ed87` | 29 |
| `third_party/arcane/docker` | `dd1cd368e9a41b85c3a154d16b1e2e62e0cc4671` | 24 |
| `third_party/arcane/streams` | `2106ec978d9f7bc3cba2aea7cc17bcdfce1e1050` | 15 |
| `third_party/arcane/updater` | `fef1384f5c64e534b1b2fdcf06c31bf947b44048` | 66 |
| `third_party/arcane/sys/bytes` | `8eb616b8403c771ea7a74025155c2c0b66a88ae7` | 5 |
| `third_party/arcane/sys/cgroup` | `dcfc4b43513b33c3bab20979b59dcb5c64f8c0f2` | 9 |
| `third_party/arcane/sys/crypto` | `79e90751a3b18a63b032a59e510947814b02da57` | 7 |
| `third_party/arcane/docker/compat` | `e76be4061fbb594e7a98b495192e5058959ec8cc` | 11 |
| `third_party/arcane/docker/convert` | `4f05acf5068d63c42b7d088acf51304bc88594ee` | 16 |

## Exclusions and deviations

Only the upstream backend, CLI, types and exact helper module ZIPs are adopted.
Arcane root automation, frontend, documentation, Docker recipes, scripts and
other unrelated root files are excluded. The existing root Arcane licence remains
byte-identical. Helper repository automation, release metadata, changelogs and the
obsolete nested workspace are excluded; helper READMEs, test fixtures, source and
all legal notices remain. Exact helper exclusions:

- `third_party/arcane/kit/.gitignore`
- `third_party/arcane/kit/cliff.toml`
- `third_party/arcane/kit/go.work`
- `third_party/arcane/kit/go.work.sum`
- `third_party/arcane/kit/CHANGELOG.md`
- `third_party/arcane/kit/Justfile`
- `third_party/arcane/kit/sys/.gitignore`
- `third_party/arcane/kit/.github/CODEOWNERS`
- `third_party/arcane/kit/.github/dependabot.yml`
- `third_party/arcane/kit/.github/workflows/ci.yml`
- `third_party/arcane/kit/.github/workflows/release.yml`
- `third_party/arcane/updater/.gitignore`
- `third_party/arcane/updater/CHANGELOG.md`
- `third_party/arcane/streams/.gitignore`
- `third_party/arcane/streams/CHANGELOG.md`
- `third_party/arcane/sys/bytes/CHANGELOG.md`
- `third_party/arcane/sys/crypto/CHANGELOG.md`
- `third_party/arcane/sys/cgroup/CHANGELOG.md`
- `third_party/arcane/builds/CHANGELOG.md`
- `third_party/arcane/builds/.gitignore`
- `third_party/arcane/docker/CHANGELOG.md`
- `third_party/arcane/docker/.gitignore`
- `third_party/arcane/docker/compat/CHANGELOG.md`
- `third_party/arcane/docker/convert/CHANGELOG.md`
- `third_party/arcane/acfs/CHANGELOG.md`
- `third_party/arcane/acfs/.gitignore`
- `third_party/arcane/acfs/.goreleaser.yaml`

The three project modules use `github.com/florianfinn/docklet-next/` without a
major-version suffix. Their local requirements use `v0.0.0`; no remote version is
selected. The root workspace selects these three modules. All consuming manifests
supply explicit relative replacements for the eleven helper modules; helpers
retain their upstream module identities. Helper-to-helper replacements follow
the new layout. The pinned Docker buildx replacement remains unchanged.

The CLI user command directory is renamed to `cli/pkg/admin/usercmd`, with its
package identifier and behaviour preserved, because the original filename
matches the publication scanner's workstation-path rule. Its header records the
original path. No product names or protocol identifiers are otherwise renamed.
Native provenance headers cover Go, module, Markdown, YAML, proto, SQL, SVG
and template source. Template comments preserve rendered output; SQL comments
preserve migration statements. SQL also has exact provenance entries because
the foundation header parser does not recognize native SQL line comments.
Exact hash-bound entries cover binary assets,
checksums, JSON, byte-sensitive golden fixtures and legal texts. The adapted Moby
buildkit and Kubernetes signal files retain Apache-2.0 provenance in addition to
the module BSD grant. Existing upstream notices are preserved.

Default hub builds embed a minimal original HTML placeholder and a nonempty
`_app` directory. Agent builds use `CGO_ENABLED=0` and `exclude_frontend`.
The two tests `TestPublishedAccessSurfaceURLsHaveFrontendRoutes` and
`TestFrontendNavigationReferencesKnownAccessSurfaces`, plus their exclusive file
lookup helpers, are excluded because their Svelte source inputs do not exist in
this repository. The backend access-policy tests and frontend HTTP behaviour test
remain. This is no claim of a working product UI.

Publication allowances are exact path/category/line-hash/count records. Each
finding was inspected in context. Reasons distinguish synthetic credentials,
parser/address fixtures, public schema/protocol/display labels, SQL identifiers,
format strings, generic runtime paths, source URLs, Go member access and the
SSRF filter's standard special-use ranges. No operational credential was found.
The agent email is an upstream synthetic actor identity; other non-example-email
findings are transport syntax or deliberately invalid validation inputs, not
personal contacts. Embedded images are the upstream logos/icons and generic
profile silhouette, not environment screenshots. No endorsement is implied.

## Builds, checks and inventories

`pnpm run lint`, `pnpm run test` and `pnpm run build` integrate all imported Go
modules. Formatting is checked with the active toolchain's `go/format`, allowing
a container-only Go installation. A small Go launcher removes Docker endpoint
variables inside the toolchain environment, after a container wrapper has used
its transport. Helper checks use `GOWORK=off` to exercise their relative
replacements. Root builds emit hub, agent and CLI binaries under ignored
`.scratch/build/`. No binary is committed or released.

The `checks` CI job uses SHA-pinned setup-go with `backend/go.mod`, caches all
module checksums, stops runner Docker services and verifies that their sockets
are absent before lint/test/build. It retains read-only repository permissions
and a bounded timeout. CLI paths now belong to the executed change scope.

The eight upstream inventory files regenerate byte-identically offline. They
remain immutable input evidence. `scripts/inventory-core.mjs` generates
`core-import.json` for the adapted source: 297 modules and 1,587 packages per
hub/agent target. Package namespace changes are expected; external dependency
versions and the measured graph membership remain unchanged. The local builds
and kit module expressions additionally record the preserved Apache-2.0 source
notices; the upstream source assessment remains unchanged. The local inventory
records all 14 manifests, their checksum files, and root workspace metadata;
root Go checks reject manifest/checksum drift. The copied upstream workspace sum
is replaced by the minimal checksum file generated for the local workspace.

All helper licence variants and the supplementary `sys/LICENSE` are recorded in
`scripts/license-policy.json`, verified by the core inventory check and covered
by exact provenance hashes. Release packaging must still assemble and inspect
the complete third-party licence bundle. The SQLite decision replaces the former removal boundary with exact compiled-file
assessment and packaged notices.


## SQLite source selection, 2026-10-16

The CGO-free Linux/amd64 hub and agent graphs each contain 273 modules and 1,489
non-standard-library packages. The current generated inventory records complete
graph hashes, module versions, local lockfile hashes, legal file hashes and
corresponding-source links. PostgreSQL providers and drivers are absent. The
[compiled notice assessment](../design/sqlite-license-assessment.md) covers the
two exact libc header files; unchanged historical Arcane inventories remain input
evidence. Candidate packaging copies all listed Go and web legal texts and checks
every extracted hash. This is not a product release or approval of other targets.
