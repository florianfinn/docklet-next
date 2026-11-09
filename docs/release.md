<!--
Source: docklet-next/issues/362
Revision: b11b3e70e671dd3c862b966b1d279251842c7319
Retrieved: 2026-11-09
License: Apache-2.0
Deviations: Original release packaging and runtime assessment.
-->

# Release packages

The release workflow builds Linux/amd64 hub and agent images from one source
commit. Runtime images use scratch, static Go binaries, embedded time zones,
the assessed CA bundle and licence texts. There is no shell or GPU utility;
GPU metrics require a custom image. ARM targets are outside this release.

Pull requests touching release inputs run a dry build at the PR head with
`<deploy/release/version.json next>-rc.<run number>`. The workflow uploads two
OCI layouts and an installation package containing the versioned Compose file,
`.env.example`, `release-manifest.json`, licence/notice texts and `SHA256SUMS`. These runs publish
nothing. The manifest records the source SHA, version, source timestamp,
release repository, index and platform digests. Review the checks and practical
acceptance evidence for the exact candidate before authorising a release.

For a local build, install the pinned web dependencies, then run from the root:

```sh
node deploy/release/build.mjs --version 1.0.0-rc.0 --out .scratch/release
node deploy/release/render.mjs --manifest .scratch/release/release-manifest.json --out .scratch/package
```

BuildKit must support OCI export with `rewrite-timestamp`. Builds are sequential:
agent first, then hub with the agent's exact index digest compiled into its
default. `SOURCE_DATE_EPOCH` defaults to the source commit timestamp; version,
revision and build time are linker stamps. The export disables attestations and
uses Linux/amd64 only. The same checkout, toolchain pins, version, epoch and
repository must produce the same OCI index digests. The override
`--release-repository registry.example.com/project/docklet-next` is for private
test builds as well as mirrors; it changes both the manifest and compiled
defaults. It never changes the release workflow's official repository.

The exact OCI layouts can be imported for smokes with
`node scripts/release-load.mjs --dir .scratch/release`. This needs a Docker
Engine using the containerd image store, Go and OpenSSL. Run both image smoke
scripts with the emitted image identities. Smokes inspect the shipped CA hash,
licences, private data directories, numeric non-root re-exec, time zones and
TLS trust using mounted trusted and untrusted test bundles. They do not use a
shell in the runtime. Repository unit tests need no Docker daemon.

## Maintainer publication

1. Complete independent review, green checks and approved practical acceptance
   for the exact release source and package. Update `version.json` for a new
   release train before tagging. Every Docklet default must have a SemVer tag
   and index digest; operator image overrides remain unrestricted.
2. Configure the GitHub `release` environment with required maintainer review
   and tag deployment restrictions. This is a repository setting, not created
   by the workflow. Confirm the token can publish the two GHCR packages and
   create a GitHub Release. Public visibility is an operator setting too.
3. Create the intended `vX.Y.Z` or `vX.Y.Z-pre` tag on an accepted commit in
   `main` history and push it only with explicit publication authorisation.
   The workflow verifies the tag's release train and main ancestry.
4. Inspect the build/package jobs and approve the `release` environment only
   for the accepted candidate. Only the publish job receives packages/contents
   write permission. It uploads the unchanged OCI blobs, platform manifest and
   index, verifies remote bytes/digests, then creates the GitHub Release.
5. Download the assets and verify `SHA256SUMS`. Retain the manifest with release
   evidence and confirm the published image index digests match it. A failed
   upload may leave untagged blobs; identical retries are safe, while a tag
   pointing to different bytes is refused. Never replace a version tag.

`deploy/release/push-oci.mjs --layout DIR --reference PIN` transfers an OCI
layout directly through the registry HTTP API. Authentication uses
`REGISTRY_BEARER_TOKEN` or `REGISTRY_USERNAME`/`REGISTRY_PASSWORD`, including
bearer challenge exchange. Keep credentials in private environment values.
HTTPS is the default; `--scheme http` is limited to loopback/private addresses
for isolated registries. Publication never uses Docker load/tag/push.

## Before deploy

Scratch omits Alpine userland and optional GPU executables. Keep local SQLite
storage and backups, the external encryption key, root-domain hosting and the
hub/agent compatibility boundary. Release packaging does not authorise a deploy
or a database upgrade. Follow the installation and upgrade guides and back up
the database before schema changes.
