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
musl toolchain. Build packages are resolved from the pinned Alpine release's APK
repositories; those repository contents are not snapshotted, so this is not a
claim of bit-for-bit reproducibility across future rebuilds.

The explicit runtime identity is UID/GID `0:0`, needed for the helper's volume
operations. The caller controls mounts, network, capabilities and lifecycle;
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
source tarballs or built images. Root Node tests check pin and protocol drift
without Docker. CI builds and exercises each architecture on its native runner.

## Publication gate

PRs and pushes to main build and test both platforms without registry write
permission. Only an explicitly pushed `tools-vMAJOR.MINOR.PATCH` tag enables the
publish job, after both platform contracts pass. The tag must point into main
history. The coordinator triggers the first publication only after maintainer
approval. Merging this workflow or an input bump does not publish an image.
Tags use stable SemVer without prerelease/build suffixes; no moving tag is pushed.
The publish job alone has `packages: write`, using only `GITHUB_TOKEN`; repository
contents remain read-only. The tested image archives pass directly to this job,
so publication does not rebuild the binaries. Failed publication can leave
platform tags or a source image; it cannot expose runtime images before their
corresponding source is available.

## Licences and corresponding source

`/usr/share/licenses/docklet-tools` carries the acfs/Kit/Arcane BSD notices,
Go standard library BSD text, Trivy Apache-2.0 and NOTICE, BusyBox GPL-2.0,
Mozilla CA MPL-2.0 notice, and the musl copyright/licence text. The provenance
inventory identifies the exact sources and licence hashes. The embedded CA bundle
also retains its upstream extraction and source notices.

Each version publishes a companion OCI source image at the same repository,
with tag `MAJOR.MINOR.PATCH-source` and platform tags ending in
`-source-amd64` / `-source-arm64`. This is complete corresponding source delivery,
not a written source offer. It contains the exact BusyBox source tarball,
platform-generated `.config`, seed flags, applet list, Dockerfile and build
scripts/inputs. Both platforms' source images are pushed before any runtime
image. Keep the source tags available for as long as the associated binaries
are distributed, and mirror source images together with runtime images.

Extract source without running the source image:

```sh
docker pull ghcr.io/florianfinn/docklet-tools:1.0.0-source-amd64
source_id=$(docker create --label t3.project=docklet-next \
  ghcr.io/florianfinn/docklet-tools:1.0.0-source-amd64 /unused)
docker cp "$source_id:/." ./tools-source
docker rm "$source_id"
```

Before deployment, select the approved SemVer image and registry digest in the
owning helper setting. Publication does not roll out images or update any running
system. Scanner databases remain runtime downloads outside this image's build
contract; the contract verifies Trivy execution without downloading a database.
