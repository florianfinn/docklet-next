// Source: getarcaneapp/arcane/types/go.mod
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; Relative local helper replacements

module github.com/florianfinn/docklet-next/types

go 1.27

require (
	github.com/compose-spec/compose-go/v2 v2.16.1
	github.com/docker/go-units v0.5.0
	github.com/moby/moby/api v1.56.1
	github.com/moby/moby/client v0.6.1
	github.com/stretchr/testify v1.12.1
	go.getarcane.app/builds v0.4.4
)

require (
	github.com/Microsoft/go-winio v0.6.3-0.20251027160822-ad3df93bed29 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.8.1 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/mattn/go-shellwords v1.0.16 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.6 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace (
	go.getarcane.app/acfs => ../third_party/arcane/acfs
	go.getarcane.app/builds => ../third_party/arcane/builds
	go.getarcane.app/docker => ../third_party/arcane/docker
	go.getarcane.app/docker/compat => ../third_party/arcane/docker/compat
	go.getarcane.app/docker/convert => ../third_party/arcane/docker/convert
	go.getarcane.app/kit => ../third_party/arcane/kit
	go.getarcane.app/streams => ../third_party/arcane/streams
	go.getarcane.app/sys/bytes => ../third_party/arcane/sys/bytes
	go.getarcane.app/sys/cgroup => ../third_party/arcane/sys/cgroup
	go.getarcane.app/sys/crypto => ../third_party/arcane/sys/crypto
	go.getarcane.app/updater => ../third_party/arcane/updater
)
