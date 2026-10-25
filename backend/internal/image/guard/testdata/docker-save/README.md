<!--
Source: docklet-next/issues/229
Revision: 474f31a45f4909c153dee30a37306fcd838678da
Retrieved: 2026-10-25
License: Apache-2.0
Deviations: Exact generated JSON metadata only; binary layer members replaced in tests.
-->

# Docker-save producer fixtures

Captured with Docker Engine 29.8.2 on 2026-10-25 using its containerd image
store. These are generated archive metadata, not extracted image filesystem
content. Every included metadata member is preserved byte-for-byte: top-level JSON,
nested indexes, manifests and configs. Attestation payloads are excluded.
`members.json` records regular tar members; layer and attestation members become
dummy bytes when tests assemble archives.
No binary layers or application files are checked in. Blob filenames remain the
producer's content digests, checked against the preserved JSON bytes by tests.

| Fixture | Saved reference | Inspected image ID |
| --- | --- | --- |
| busybox | `busybox:1.37` | `sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e` |
| alpine | `alpine:3.22` | `sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8` |
| nginx | `nginx:1.29-alpine` | `sha256:5616878291a2eed594aee8db4dade5878cf7edcb475e59193904b198d9b830de` |
| local | `docklet-tools:0.0.0-r1` | `sha256:7c4097ec23574e0a0d3aa1b9abf320dc68f9c7e3d3c1074742b9410440092b49` |

All four images were already present. `docklet-tools:0.0.0-r1` is a locally
built image; the others were pulled registry images. No container was started
or changed for capture. The five acceptance cases are busybox tar, alpine tar,
alpine gzip, nginx tar and the locally built image tar.

To reproduce a capture, use the saved references on an equivalent Engine,
verify their recorded image IDs and extract only import metadata:

```sh
docker version --format '{{.Server.Version}}'
docker image inspect busybox:1.37 --format '{{.Id}}'
docker image save --output busybox.tar busybox:1.37
python3 extract.py busybox.tar busybox
gzip -c alpine.tar > alpine.tar.gz
```

Repeat save/extraction for alpine, nginx and the local image. Archive validation
reads the exact top-level descriptor keys `io.containerd.image.name` and
`org.opencontainers.image.ref.name`, plus legacy `manifest.json` RepoTags.
Annotations such as nested `org.opencontainers.image.base.name` and
`vnd.docker.reference.*` describe provenance; they do not assign imported names.
The busybox/alpine metadata retains these real annotations and absent platform
manifest descriptors, so restoring substring-based annotation rejection fails
the producer acceptance tests. Reserved references are changed independently in
each actual carrier for each of the five acceptance cases.

The member catalogues and extraction helper are Apache-2.0. Config history can
contain upstream Dockerfile build instructions. The nginx config retains
instructions matching docker-nginx's
[`mainline/alpine-slim/Dockerfile`](https://github.com/nginx/docker-nginx/blob/71081b25390771f6b1275ddf7c73c965f304493f/mainline/alpine-slim/Dockerfile)
and
[`mainline/alpine/Dockerfile`](https://github.com/nginx/docker-nginx/blob/71081b25390771f6b1275ddf7c73c965f304493f/mainline/alpine/Dockerfile)
at revision `71081b25390771f6b1275ddf7c73c965f304493f`, under BSD-2-Clause.
The complete F5 copyright notice, conditions and disclaimer are preserved in
[`LICENSES/BSD-2-Clause-docker-nginx.txt`](../../../../../../LICENSES/BSD-2-Clause-docker-nginx.txt).
Metadata stays byte-identical to the saved archive. No image executable or
layer payload is included.
