// Source: getarcaneapp/kit/streams/go.mod
// Revision: 2786ddf03ec35e01655d19b271591436d0057f79
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements

module go.getarcane.app/streams

go 1.27

require github.com/moby/moby/api v1.56.1

require (
	github.com/docker/go-units v0.5.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
)

retract v0.4.1 // published without agg/hub.go; breaks consumers of agg.Hub

replace (
	go.getarcane.app/acfs => ../acfs
	go.getarcane.app/builds => ../builds
	go.getarcane.app/docker => ../docker
	go.getarcane.app/docker/compat => ../docker/compat
	go.getarcane.app/docker/convert => ../docker/convert
	go.getarcane.app/kit => ../kit
	go.getarcane.app/sys/bytes => ../sys/bytes
	go.getarcane.app/sys/cgroup => ../sys/cgroup
	go.getarcane.app/sys/crypto => ../sys/crypto
	go.getarcane.app/updater => ../updater
)
