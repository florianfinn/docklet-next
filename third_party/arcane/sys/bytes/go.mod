// Source: getarcaneapp/kit/sys/bytes/go.mod
// Revision: 60f275d8746e5aff375a298b6fef1c0a2f373296
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements

module go.getarcane.app/sys/bytes

go 1.27

replace (
	go.getarcane.app/acfs => ../../acfs
	go.getarcane.app/builds => ../../builds
	go.getarcane.app/docker => ../../docker
	go.getarcane.app/docker/compat => ../../docker/compat
	go.getarcane.app/docker/convert => ../../docker/convert
	go.getarcane.app/kit => ../../kit
	go.getarcane.app/streams => ../../streams
	go.getarcane.app/sys/cgroup => ../cgroup
	go.getarcane.app/sys/crypto => ../crypto
	go.getarcane.app/updater => ../../updater
)
