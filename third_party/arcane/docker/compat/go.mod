// Source: getarcaneapp/kit/docker/compat/go.mod
// Revision: de96f232f840ee55de637f1178f8cb890de9a642
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements

module go.getarcane.app/docker/compat

go 1.27

require (
	github.com/moby/moby/api v1.56.1
	github.com/moby/moby/client v0.6.1
)

require (
	github.com/Microsoft/go-winio v0.6.3-0.20251027160822-ad3df93bed29 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.8.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace (
	go.getarcane.app/acfs => ../../acfs
	go.getarcane.app/builds => ../../builds
	go.getarcane.app/docker => ..
	go.getarcane.app/docker/convert => ../convert
	go.getarcane.app/kit => ../../kit
	go.getarcane.app/streams => ../../streams
	go.getarcane.app/sys/bytes => ../../sys/bytes
	go.getarcane.app/sys/cgroup => ../../sys/cgroup
	go.getarcane.app/sys/crypto => ../../sys/crypto
	go.getarcane.app/updater => ../../updater
)
