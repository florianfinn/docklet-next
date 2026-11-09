# Source: docklet-next/docs/design/sqlite-database.md
# Revision: b3607be5144101152c8eb7738d389e8c73986fba
# Retrieved: 2026-10-16
# License: Apache-2.0
# Deviations: Original CGO-free hub image with embedded production UI and licence bundles; stamped scratch hub and agent runtimes with assessed CA roots.
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

FROM ${GO_IMAGE} AS build-source
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0 GOFLAGS=-p=2 GOCACHE=/var/cache/go-build
ARG DOCKLET_VERSION=dev
ARG DOCKLET_REVISION=unknown
ARG SOURCE_DATE_EPOCH=0
ARG DOCKLET_LDFLAGS

FROM build-source AS agent-build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/var/cache/go-build \
    flags="${DOCKLET_LDFLAGS:--X github.com/florianfinn/docklet-next/backend/internal/config.Version=${DOCKLET_VERSION}}" && \
    go build -buildvcs=false -trimpath -ldflags "$flags" -tags=exclude_frontend -o /out/docklet-agent ./backend/cmd && \
    go run scripts/core-license-files.go bundle /out/licenses

FROM build-source AS build
RUN rm -rf backend/frontend/dist
COPY --from=web /src/web/dist ./backend/frontend/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/var/cache/go-build \
    flags="${DOCKLET_LDFLAGS:--X github.com/florianfinn/docklet-next/backend/internal/config.Version=${DOCKLET_VERSION}}" && \
    go build -buildvcs=false -trimpath -ldflags "$flags" -o /out/docklet-hub ./backend/cmd && \
    go run scripts/core-license-files.go bundle /out/licenses

FROM ${RUNTIME_IMAGE} AS runtime-files
RUN echo "028fca5150ee1915c4474489985d55c54fceaa680c754cc8d1301734f01b2232  /etc/ssl/certs/ca-certificates.crt" | sha256sum -c - && \
    mkdir -p /runtime/tmp /runtime/app/data /runtime/var/lib/docklet/backups && \
    chmod 1777 /runtime/tmp && chmod 0700 /runtime/app/data /runtime/var/lib/docklet/backups

FROM scratch AS agent
LABEL io.docklet.system=agent
WORKDIR /app
COPY --from=runtime-files /runtime/ /
COPY --from=runtime-files /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=agent-build /out/docklet-agent /usr/local/bin/docklet-agent
COPY --from=agent-build /out/licenses /usr/share/licenses/docklet-next
COPY LICENSES/MPL-2.0-Mozilla.txt LICENSES/NOTICE-Mozilla-CA.txt /usr/share/licenses/docklet-next/ca/
ENV AGENT_MODE=true EDGE_AGENT=false LISTEN=0.0.0.0 PORT=3552
EXPOSE 3552
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/usr/local/bin/docklet-agent", "health"]
ENTRYPOINT ["/usr/local/bin/docklet-agent"]

FROM scratch AS hub
LABEL io.docklet.system=hub
WORKDIR /app
COPY --from=runtime-files /runtime/ /
COPY --from=runtime-files /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/docklet-hub /usr/local/bin/docklet-hub
COPY --from=build /out/licenses /usr/share/licenses/docklet-next
COPY --from=web /out/licenses/web /usr/share/licenses/docklet-next/web
COPY LICENSES/MPL-2.0-Mozilla.txt LICENSES/NOTICE-Mozilla-CA.txt /usr/share/licenses/docklet-next/ca/
ENV DATABASE_URL=file:/app/data/docklet.db LISTEN=0.0.0.0 PORT=3552
EXPOSE 3552
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/usr/local/bin/docklet-hub", "health"]
ENTRYPOINT ["/usr/local/bin/docklet-hub"]
