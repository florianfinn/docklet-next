# Source: docklet-next/docs/design/hub-backup.md
# Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
# Retrieved: 2026-10-15
# License: Apache-2.0
# Deviations: Original pinned Hub image for issue #32.
ARG GO_IMAGE=golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
ARG RUNTIME_IMAGE=postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873
ARG IMAGE_VERSION=0.0.0
ARG IMAGE_SOURCE=https://github.com/florianfinn/docklet-next
ARG IMAGE_REVISION=unknown
FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY backend/ backend/
COPY types/ types/
COPY cli/ cli/
COPY third_party/ third_party/
RUN cd backend && CGO_ENABLED=0 go build -o /out/docklet-hub ./cmd
FROM ${RUNTIME_IMAGE}
ARG IMAGE_VERSION
ARG IMAGE_SOURCE
ARG IMAGE_REVISION
COPY --from=build /out/docklet-hub /usr/local/bin/docklet-hub
LABEL org.opencontainers.image.title="Docklet Next Hub" \
      org.opencontainers.image.description="Docklet Next Hub with PostgreSQL 18 backup tools" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${IMAGE_VERSION}" \
      org.opencontainers.image.source="${IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${IMAGE_REVISION}"
RUN mkdir -p /var/lib/docklet/backups && chown postgres:postgres /var/lib/docklet/backups && chmod 0700 /var/lib/docklet/backups
ENV DOCKLET_BACKUP_DIR=/var/lib/docklet/backups
VOLUME ["/var/lib/docklet/backups"]
EXPOSE 3552
USER postgres
ENTRYPOINT ["/usr/local/bin/docklet-hub"]
