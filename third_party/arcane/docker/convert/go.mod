// Source: getarcaneapp/kit/docker/convert/go.mod
// Revision: ad1f8d7e16f4bfaf4f6dae6b35d9ea1e3ef961b5
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements

module go.getarcane.app/docker/convert

go 1.27

require (
	github.com/compose-spec/compose-go/v2 v2.16.1
	github.com/docker/go-units v0.5.0
	github.com/mattn/go-shellwords v1.0.16
	go.yaml.in/yaml/v4 v4.0.0-rc.6
)

require (
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.8.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace (
	go.getarcane.app/acfs => ../../acfs
	go.getarcane.app/builds => ../../builds
	go.getarcane.app/docker => ..
	go.getarcane.app/docker/compat => ../compat
	go.getarcane.app/kit => ../../kit
	go.getarcane.app/streams => ../../streams
	go.getarcane.app/sys/bytes => ../../sys/bytes
	go.getarcane.app/sys/cgroup => ../../sys/cgroup
	go.getarcane.app/sys/crypto => ../../sys/crypto
	go.getarcane.app/updater => ../../updater
)
