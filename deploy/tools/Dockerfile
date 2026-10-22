# Source: getarcaneapp/tools/Dockerfile
# Revision: 01903271fa1297cc3f61e95a19d0becb5ac535fa
# Retrieved: 2026-10-22
# License: BSD-3-Clause
# Deviations: Offline vendored acfs; two platforms; immutable input pins; source export; packaged notices.
ARG GO_IMAGE
ARG BUILD_IMAGE
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
    cp /usr/local/go/LICENSE /out/usr/share/licenses/docklet-tools/Go-BSD-3-Clause.txt && \
    cp /src/acfs/LICENSE /out/usr/share/licenses/docklet-tools/acfs-BSD-3-Clause.txt && \
    cp /src/kit/LICENSE /out/usr/share/licenses/docklet-tools/kit-BSD-3-Clause.txt

FROM ${BUILD_IMAGE} AS busybox
ARG BUSYBOX_VERSION
ARG BUSYBOX_SHA256
RUN apk add --no-cache build-base bzip2 curl linux-headers perl
WORKDIR /work
COPY deploy/tools /recipe
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
    yes n | make oldconfig && make -j2 && \
    install -Dm755 busybox /out/bin/busybox && \
    mkdir -p /out/tmp /out/var/cache/docklet-tools /out/usr/share/licenses/docklet-tools /source && \
    while IFS= read -r applet; do \
      case "$applet" in ''|'#'*) continue ;; esac; \
      ln -s /bin/busybox "/out/bin/$applet"; \
    done < /recipe/applets.txt && chmod 1777 /out/tmp && \
    cp LICENSE /out/usr/share/licenses/docklet-tools/BusyBox-GPL-2.0.txt && \
    cp /work/busybox.tar.bz2 /source/busybox-$BUSYBOX_VERSION.tar.bz2 && \
    cp .config /source/.config && cp /recipe/* /source/

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

FROM scratch AS corresponding-source
COPY --from=busybox /source/ /

FROM scratch AS runtime
ARG RUNTIME_USER
COPY --from=busybox /out/ /
COPY --from=downloads /out/ /
COPY --from=acfs /out/ /
COPY LICENSES/BSD-3-Clause-Arcane.txt /usr/share/licenses/docklet-tools/Arcane-BSD-3-Clause.txt
ENV PATH=/usr/local/bin:/bin SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt TMPDIR=/tmp XDG_CACHE_HOME=/var/cache/docklet-tools
USER ${RUNTIME_USER}
ENTRYPOINT []
CMD ["/bin/sh"]
