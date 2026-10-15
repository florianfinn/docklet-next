# Source: docklet-next/docs/design/hub-backup.md
# Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
# Retrieved: 2026-10-15
# License: Apache-2.0
# Deviations: Original pinned Hub image for issue #32.
ARG GO_IMAGE=golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
ARG RUNTIME_IMAGE=postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873
FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY backend/ backend/
COPY types/ types/
COPY cli/ cli/
COPY third_party/ third_party/
RUN cd backend && CGO_ENABLED=0 go build -o /out/docklet-hub ./cmd
FROM ${RUNTIME_IMAGE}
COPY --from=build /out/docklet-hub /usr/local/bin/docklet-hub
RUN mkdir -p /var/lib/docklet/backups && chmod 0700 /var/lib/docklet/backups
ENV DOCKLET_BACKUP_DIR=/var/lib/docklet/backups
VOLUME ["/var/lib/docklet/backups"]
EXPOSE 3552
ENTRYPOINT ["/usr/local/bin/docklet-hub"]
