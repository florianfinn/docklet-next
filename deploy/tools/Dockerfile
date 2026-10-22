# Source: getarcaneapp/tools/Dockerfile
# Revision: 01903271fa1297cc3f61e95a19d0becb5ac535fa
# Retrieved: 2026-10-22
# License: BSD-3-Clause
# Deviations: Local acfs replaces cloning (protocol alignment); amd64/arm64 only (supported targets).
# Pinned Go Bookworm 1.27.2 replaces Alpine Go 1.27.0 (repository toolchain/offline build).
# Alpine 3.22.6 replaces 3.24 (retained reviewed musl 1.2.5/toolchain); build packages are inventoried.
# Dated checksum-pinned curl CA PEM replaces Alpine CA output (stable source/version).
# Neutral /var/cache/docklet-tools replaces the upstream home cache (publication policy).
# Explicit root user and shell CMD support volume ownership access and direct helper commands.
# Omit the unpinned frontend directive (daemon frontend) and upstream product labels (Docklet ownership).
# Add Node licence assembly, exact dependency evidence, source export/digests and OCI source/revision labels.
ARG GO_IMAGE
ARG BUILD_IMAGE
ARG NODE_IMAGE
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS acfs
ARG TARGETARCH
ARG ACFS_VERSION
ARG ACFS_REVISION
WORKDIR /src/acfs
COPY third_party/arcane/acfs /src/acfs
COPY third_party/arcane/kit /src/kit
ENV CGO_ENABLED=0 GOWORK=off GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2
RUN --network=none case "$TARGETARCH" in amd64|arm64) ;; *) exit 1 ;; esac && \
    GOOS=linux GOARCH=$TARGETARCH go build -mod=mod -p=1 -trimpath -buildvcs=false \
    -ldflags "-s -w -X go.getarcane.app/acfs/internal/version.Version=$ACFS_VERSION -X go.getarcane.app/acfs/internal/version.Revision=$ACFS_REVISION" \
    -o /out/usr/local/bin/acfs ./cmd && \
    mkdir -p /out/usr/share/licenses/docklet-tools && \
    GOOS=linux GOARCH=$TARGETARCH go list -mod=mod -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./cmd | sed '/^$/d' > /out/usr/share/licenses/docklet-tools/acfs-packages.txt && \
    go version -m /out/usr/local/bin/acfs > /out/usr/share/licenses/docklet-tools/acfs-buildinfo.txt && \
    cp /usr/local/go/LICENSE /out/usr/share/licenses/docklet-tools/Go-BSD-3-Clause.txt && \
    cp /src/acfs/LICENSE /out/usr/share/licenses/docklet-tools/acfs-BSD-3-Clause.txt && \
    cp /src/kit/LICENSE /out/usr/share/licenses/docklet-tools/kit-BSD-3-Clause.txt

FROM ${BUILD_IMAGE} AS busybox
ARG BUSYBOX_VERSION
ARG BUSYBOX_SHA256
ARG MUSL_PACKAGE
ARG GCC_PACKAGE
RUN apk add --no-cache build-base bzip2 curl linux-headers perl "musl-dev=$MUSL_PACKAGE" "gcc=$GCC_PACKAGE"
WORKDIR /work
COPY deploy/tools/Dockerfile deploy/tools/build.mjs deploy/tools/inputs.env deploy/tools/busybox.config deploy/tools/applets.txt /recipe/
RUN curl -fsSL "https://busybox.net/downloads/busybox-$BUSYBOX_VERSION.tar.bz2" -o busybox.tar.bz2 && \
    echo "$BUSYBOX_SHA256  busybox.tar.bz2" | sha256sum -c - && \
    mkdir source && tar -xjf busybox.tar.bz2 -C source --strip-components=1
WORKDIR /work/source
RUN make allnoconfig && \
    while IFS='=' read -r key value; do \
      case "$key" in CONFIG_*) ;; *) continue ;; esac; \
      sed -i -e "/^$key=.*/d" -e "/^# $key is not set/d" .config; \
      printf '%s=%s\n' "$key" "$value" >> .config; \
    done < /recipe/busybox.config && \
    yes n | make oldconfig && make -j2 EXTRA_LDFLAGS=-Wl,-Map,/work/busybox.map && \
    install -Dm755 busybox /out/bin/busybox && \
    mkdir -p /out/tmp /out/var/cache/docklet-tools /out/usr/share/licenses/docklet-tools /source && \
    while IFS= read -r applet; do \
      case "$applet" in ''|'#'*) continue ;; esac; \
      ln -s /bin/busybox "/out/bin/$applet"; \
    done < /recipe/applets.txt && chmod 1777 /out/tmp && \
    cp LICENSE /out/usr/share/licenses/docklet-tools/BusyBox-GPL-2.0.txt && \
    cp .config /out/usr/share/licenses/docklet-tools/busybox.config && \
    cp /recipe/applets.txt /out/usr/share/licenses/docklet-tools/applets.txt && \
    cp /work/busybox.tar.bz2 /source/busybox-$BUSYBOX_VERSION.tar.bz2 && \
    cp .config /source/.config && cp /recipe/* /source/ && apk info -v > /source/toolchain-packages.txt && \
    sha256sum busybox > /source/busybox.sha256 && cp /work/busybox.map /source/busybox.map && \
    cp /source/toolchain-packages.txt /source/busybox.sha256 /source/busybox.map /out/usr/share/licenses/docklet-tools/

FROM --platform=$BUILDPLATFORM ${BUILD_IMAGE} AS downloads
ARG TARGETARCH
ARG TRIVY_VERSION
ARG TRIVY_AMD64_SHA256
ARG TRIVY_ARM64_SHA256
ARG CA_VERSION
ARG CA_SHA256
RUN apk add --no-cache curl tar
WORKDIR /work
RUN case "$TARGETARCH" in \
      amd64) archive=64bit; checksum=$TRIVY_AMD64_SHA256 ;; \
      arm64) archive=ARM64; checksum=$TRIVY_ARM64_SHA256 ;; \
      *) exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/aquasecurity/trivy/releases/download/v$TRIVY_VERSION/trivy_${TRIVY_VERSION}_Linux-$archive.tar.gz" -o trivy.tar.gz && \
    echo "$checksum  trivy.tar.gz" | sha256sum -c - && \
    tar -xzf trivy.tar.gz trivy LICENSE && \
    install -Dm755 trivy /out/usr/local/bin/trivy && \
    install -Dm644 LICENSE /out/usr/share/licenses/docklet-tools/Trivy-Apache-2.0.txt && \
    curl -fsSL "https://curl.se/ca/cacert-$CA_VERSION.pem" -o cacert.pem && \
    echo "$CA_SHA256  cacert.pem" | sha256sum -c - && \
    install -Dm644 cacert.pem /out/etc/ssl/certs/ca-certificates.crt

FROM acfs AS inspected
COPY --from=downloads /out/usr/local/bin/trivy /tmp/trivy
RUN --network=none go version -m /tmp/trivy > /out/usr/share/licenses/docklet-tools/trivy-buildinfo.txt

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS legal
ARG TARGETARCH
ARG TOOLS_VERSION
ARG SOURCE_REVISION
ARG IMAGE_REPOSITORY
WORKDIR /src
COPY docs/provenance/tools-image.json docs/provenance/tools-trivy.json docs/provenance/tools-release.json docs/provenance/tools-embedded.json ./docs/provenance/
COPY LICENSES/tools ./LICENSES/tools
COPY deploy/tools/assemble-licenses.mjs deploy/tools/license-contract.mjs deploy/tools/build.mjs deploy/tools/inputs.env ./deploy/tools/
COPY --from=inspected /out/usr/share/licenses/docklet-tools /evidence
COPY --from=busybox /out/usr/share/licenses/docklet-tools /evidence
COPY --from=downloads /out/usr/local/bin/trivy /trivy
RUN --network=none node deploy/tools/assemble-licenses.mjs /out "$TARGETARCH" "$TOOLS_VERSION" "$SOURCE_REVISION" "$IMAGE_REPOSITORY"

FROM scratch AS corresponding-source
ARG SOURCE_REVISION
ARG TOOLS_VERSION
LABEL org.opencontainers.image.source=https://github.com/florianfinn/docklet-next org.opencontainers.image.revision=$SOURCE_REVISION org.opencontainers.image.version=$TOOLS_VERSION
COPY --from=busybox /source/ /
COPY LICENSE /Apache-2.0.txt
COPY LICENSES/BSD-3-Clause-Arcane.txt /Arcane-BSD-3-Clause.txt
COPY third_party/arcane/kit/LICENSE /Arcane-Tools-BSD-3-Clause.txt

FROM scratch AS runtime
ARG SOURCE_REVISION
ARG TOOLS_VERSION
LABEL org.opencontainers.image.source=https://github.com/florianfinn/docklet-next org.opencontainers.image.revision=$SOURCE_REVISION org.opencontainers.image.version=$TOOLS_VERSION
ARG RUNTIME_USER
COPY --from=busybox /out/ /
COPY --from=downloads /out/ /
COPY --from=inspected /out/ /
COPY --from=legal /out/ /usr/share/licenses/docklet-tools/
COPY LICENSES/BSD-3-Clause-Arcane.txt /usr/share/licenses/docklet-tools/Arcane-BSD-3-Clause.txt
COPY third_party/arcane/kit/LICENSE /usr/share/licenses/docklet-tools/Arcane-Tools-BSD-3-Clause.txt
COPY LICENSES/NOTICE-Trivy.txt /usr/share/licenses/docklet-tools/Trivy-NOTICE.txt
COPY LICENSES/MPL-2.0-Mozilla.txt /usr/share/licenses/docklet-tools/CA-MPL-2.0.txt
COPY LICENSES/MIT-musl.txt /usr/share/licenses/docklet-tools/musl-COPYRIGHT.txt
COPY LICENSE /usr/share/licenses/docklet-tools/Docklet-Apache-2.0.txt
COPY deploy/tools/CA-NOTICE.txt /usr/share/licenses/docklet-tools/CA-NOTICE.txt
COPY deploy/tools/inputs.env /usr/share/licenses/docklet-tools/inputs.env
ENV PATH=/usr/local/bin:/bin SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt TMPDIR=/tmp XDG_CACHE_HOME=/var/cache/docklet-tools
USER ${RUNTIME_USER}
ENTRYPOINT []
CMD ["/bin/sh"]
