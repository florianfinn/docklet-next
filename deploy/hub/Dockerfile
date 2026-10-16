# Source: docklet-next/docs/design/sqlite-database.md
# Revision: b3607be5144101152c8eb7738d389e8c73986fba
# Retrieved: 2026-10-16
# License: Apache-2.0
# Deviations: Original CGO-free hub image with embedded production UI and licence bundles.
ARG NODE_IMAGE=node:24.11.1-bookworm-slim@sha256:48abc13a19400ca3985071e287bd405a1d99306770eb81d61202fb6b65cf0b57
ARG GO_IMAGE=golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
ARG RUNTIME_IMAGE=alpine:3.22.6@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8
FROM ${NODE_IMAGE} AS web
WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./web/
RUN corepack enable && corepack prepare pnpm@11.21.0 --activate && pnpm --dir web install --frozen-lockfile
COPY web ./web
COPY scripts/package-web-licenses.mjs ./scripts/package-web-licenses.mjs
COPY LICENSES ./LICENSES
RUN pnpm --dir web run build && node scripts/package-web-licenses.mjs /out/licenses/web

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY . .
RUN rm -rf backend/frontend/dist
COPY --from=web /src/web/dist ./backend/frontend/dist
ENV CGO_ENABLED=0 GOFLAGS=-p=4 GOCACHE=/var/cache/go-build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/var/cache/go-build go build -trimpath -o /out/docklet-hub ./backend/cmd && \
    go build -trimpath -tags=exclude_frontend -o /out/docklet-agent ./backend/cmd && \
    go run scripts/core-license-files.go bundle /out/licenses

FROM ${RUNTIME_IMAGE}
WORKDIR /app
COPY --from=build /out/docklet-hub /usr/local/bin/docklet-hub
COPY --from=build /out/licenses /usr/share/licenses/docklet-next
COPY --from=web /out/licenses/web /usr/share/licenses/docklet-next/web
RUN mkdir -p /app/data /var/lib/docklet/backups && chmod 0700 /app/data /var/lib/docklet/backups && test -s /etc/ssl/certs/ca-certificates.crt
ENV DATABASE_URL=file:/app/data/docklet.db LISTEN=0.0.0.0 PORT=3552
EXPOSE 3552
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/usr/local/bin/docklet-hub", "health"]
ENTRYPOINT ["/usr/local/bin/docklet-hub"]
