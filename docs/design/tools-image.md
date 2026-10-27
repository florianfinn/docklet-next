# Tools helper image

Docklet builds `ghcr.io/florianfinn/docklet-tools` from the adopted
[Arcane tools recipe](https://github.com/getarcaneapp/tools/tree/01903271fa1297cc3f61e95a19d0becb5ac535fa).
The scratch image contains static BusyBox with the upstream flags and 14 applets,
Trivy, the Mozilla CA bundle, and acfs built from this repository's exact local
source. It supports only `linux/amd64` and `linux/arm64`. The acfs version and
protocol must match the vendored metadata; the helper never clones another acfs
release. Go builds use local acfs/kit replacements, `GOWORK=off`, `GOPROXY=off`,
and a build step with networking disabled. No external Go packages are compiled.

`deploy/tools/inputs.env` is the single version, base-image digest and download
checksum input. `busybox.config` is the seed configuration; the complete generated
`.config` is produced by each platform build. BusyBox uses the native Alpine
musl toolchain. Its musl-dev/GCC package revisions are explicitly pinned and
checked against the source inventory. Other build packages are resolved from the pinned Alpine release's APK
repositories; those repository contents are not snapshotted, so this is not a
claim of bit-for-bit reproducibility across future rebuilds.

The helper runs as root by design: UID/GID `0:0` is required to access volume
contents owned by root. The contract asserts the configured and effective root
identity, root-owned created files and writable scanner cache. Non-root operation
is outside the supported contract. The caller controls mounts, network, capabilities and lifecycle;
this image grants no Docker socket access itself. Temporary files use `/tmp`
and Trivy's default cache uses `/var/cache/docklet-tools`.

## Build and update

From the repository root with Docker buildx and Node installed:

```sh
node deploy/tools/build.mjs --platform linux/amd64 --load --tag docklet-tools:0.0.0
node deploy/tools/contract.mjs docklet-tools:0.0.0
node deploy/tools/build.mjs --platform linux/amd64 --target corresponding-source \
  --output type=local,dest=tools-source
```

A bump PR changes the input pins, verifies downloaded hashes against upstream
release evidence, refreshes affected legal files/provenance and tests both
platforms. Changes to acfs first follow its owning source adoption procedure;
then update the helper metadata. BusyBox updates must regenerate and inspect the
full `.config` and applet list for both architectures. Do not commit downloaded
source tarballs or built images. Root Node tests check pin and protocol drift, exact dependency coverage and
required notices without Docker. CI builds and exercises each architecture on its native runner.

The source-image contract also compares the exported generated configuration
against the runtime copy, verifies the exact tarball and BusyBox executable
hashes, and checks the build recipe and toolchain package versions. The source
image records the built executable digest; correspondence is verified independently
of BuildKit cache reuse across the two invocations. Run it after loading a `corresponding-source` image:

```sh
node deploy/tools/source-contract.mjs docklet-tools:0.0.0 docklet-tools-source:0.0.0
```

## Publication gate

PRs and pushes to main build and test both platforms without registry write
permission. Only an explicitly pushed `tools-vMAJOR.MINOR.PATCH` tag enables the
publish job, after both platform contracts pass. The tag must point into main
history. The coordinator triggers the first publication only after maintainer
approval. Merging this workflow or an input bump does not publish an image.
Tags use stable SemVer without prerelease/build suffixes. Before any push, a
fail-closed preflight refuses an existing runtime/source/platform tag, a matching
tools release, or a `docklet-tools-VERSION*` / `tools-VERSION*` release asset.
Authentication/network errors also refuse publication. A per-tag concurrency
lock serializes publisher runs. Failed partial publication reserves that version;
use a fresh version after investigating it. Re-running a release cannot overwrite
its source or binaries. Registry administration outside this workflow remains an
operator responsibility.
The publish job alone has `packages: write`, using only `GITHUB_TOKEN`; repository
contents remain read-only. The tested image archives pass directly to this job,
so publication does not rebuild the binaries. Failed publication can leave
platform tags or a source image; it cannot expose runtime images before their
corresponding source is available.

## First publication and release default

Release tag `tools-v0.1.0` publishes revision
`ee89b810ab4775dba5415b35a44ddc21c26b5cf1`, with acfs v0.6.2,
ACFS protocol 2 and Trivy 0.75.0. The published references are:

- Runtime index (the hub/agent release default):
  `ghcr.io/florianfinn/docklet-tools:0.1.0@sha256:d387beb35707c5634dba1630b4153c5bfe0bd4529025297ce7c024c2f2fe9602`.
- `linux/amd64` runtime digest:
  `sha256:c69b8fa23b4cae0bd1d9f247977e8f0fa7ad9b00df4961f6bf8f735073954a64`.
- `linux/arm64` runtime digest:
  `sha256:b18283f0c9a0eb343585d3cc7007175fb5501815c8aa11f932183c1f0b754d59`.
- Corresponding-source index:
  `ghcr.io/florianfinn/docklet-tools:0.1.0-source@sha256:35f88495598cbbc7dfdf4f82c44de7a9499379f24909051d386880aa93986903`.

For the next version, merge the reviewed input/version bump PR first, then
publish its `tools-vMAJOR.MINOR.PATCH` tag with explicit maintainer approval.
Verify both platform contracts and the source-first publication, record their
registry digests, then merge a separate pin PR updating `DefaultToolsImage`,
its tests and default documentation to the new SemVer plus index digest.
Do not extend the closed settings retirement list: explicit rollback pins must
survive startup. Publication and pinning each precede explicit operator rollout.

`toolsImage` stores a complete per-environment override; empty follows the
running hub or agent release default. `TOOLS_IMAGE`, `TOOLS_IMAGE_FILE` or
`TOOLS_IMAGE__FILE` can override it locally, and an agent-local value wins over
hub sync. To mirror images, preserve both runtime platforms and corresponding
source, then configure the mirror's complete reference. Overrides can also pin
an older release and are retained across restart.

## Licences and corresponding source

`/usr/share/licenses/docklet-tools` carries the acfs/Kit/Arcane BSD notices,
Go standard library BSD text, Trivy Apache-2.0 and NOTICE, BusyBox GPL-2.0,
Mozilla CA MPL-2.0 notice, musl copyright/grants and GCC runtime exception texts.
The image ships the verbatim BusyBox GPL text, including page separators; the
Git reference normalizes those separators for repository publication guards.
The provenance inventory identifies exact sources, copyright/NOTICE bytes and
hashes. Legal text payloads use JSON strings to preserve original CRLF/form-feed
bytes without altering the licence; image assembly restores and hashes them. The embedded CA bundle
also retains its upstream extraction and source notices.

Trivy's exact release archives are checked for both architectures. Their Go
build metadata identifies 377 linked dependencies each; the upstream SBOM lists
460 modules and differs from the actual binaries in two module versions. The
inventory covers the complete union: 462 exact module/version records. All root
and nested runtime licences/NOTICES travel under `trivy/`, with immutable source
references and MPL Source Code Form pointers in `trivy/inventory.json`. SBOM
licence labels are retained as evidence, but actual licence texts determine the
selected grants. Documentation, branding and separate container-image notices
are excluded with hash-bound reasons. Generated libc header notices are retained
for both architectures with the exact-file LGPL header-use/GCC runtime exception
assessment; no linked glibc implementation is asserted.

The acfs assessment freezes its seven non-stdlib package paths and compiled
source/manifests; new imports require a refreshed inventory. BusyBox records a
static link map and the exact musl/GCC versions. Unknown external archives fail
assembly. All musl copyright/grant blocks are conservatively included alongside
the compiler runtime GPL text and GCC Runtime Library Exception 3.1. Both the
build and runtime contracts verify the dependency evidence and packaged hashes.

A Trivy bump must replace `tools-release.json` (exact release SBOM identity,
per-architecture binary/archive hashes, Go build-info module lists), retrieve the
corresponding legal evidence, refresh `tools-trivy.json` and the text payloads,
and update their hash references in `tools-image.json`. Do not infer copyright
from an SPDX label. Obtain legal files from immutable VCS revisions or the
existing module cache; generated SDKs can use HTTP Range reads of only the ZIP
metadata/legal entries. Go build module downloads remain disabled. Changes to
BusyBox, musl, GCC or local acfs likewise require refreshed `tools-embedded.json`
evidence and corresponding legal text payloads. Root tests reject incomplete
updates, and image assembly performs the same checks without networking.

The runtime's OCI source/revision/version labels and `SOURCE.txt` identify the
corresponding source tag and extraction procedure. Release builds record the
exact Git revision; local modified trees are explicitly labelled `-dirty` with
an unpublished `development` version.

Each version publishes a companion OCI source image at the same repository,
with tag `MAJOR.MINOR.PATCH-source` and platform tags ending in
`-source-amd64` / `-source-arm64`. This is complete corresponding source delivery,
not a written source offer. It contains the exact BusyBox source tarball,
platform-generated `.config`, seed flags, applet list, Dockerfile and build
scripts/inputs and the exact installed build-package manifest. Both platforms' source images are pushed before any runtime
image. Keep the source tags available for as long as the associated binaries
are distributed, and mirror source images together with runtime images.

Extract source without running the source image:

```sh
docker pull ghcr.io/florianfinn/docklet-tools:1.0.0-source-amd64
source_id=$(docker create \
  ghcr.io/florianfinn/docklet-tools:1.0.0-source-amd64 /unused)
docker cp "$source_id:/." ./tools-source
docker rm "$source_id"
```

Before deployment, select the approved SemVer image and registry digest in the
owning helper setting. Publication does not roll out images or update any running
system. Scanner databases remain runtime downloads outside this image's build
contract; the contract verifies Trivy execution without downloading a database.

Vulnerability scanning mounts its persistent owner-specific cache at
`/var/cache/docklet-tools` and uses `/var/cache/docklet-tools/trivy`. An anonymous
`/work` volume holds temporary downloads and the report; `TMPDIR=/work/tmp` is
necessary for large cold Java DB downloads. The scanner targets local Docker IDs
through the daemon socket with `--image-src docker`, then removes its container
and anonymous volumes. See [vulnerability scanning](vulnerability-scanning.md).
