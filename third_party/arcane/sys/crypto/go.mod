// Source: getarcaneapp/kit/sys/crypto/go.mod
// Revision: e2a909ce45597a3c101dca4d218baef4f89bb643
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements; functional cut: module tidy.

module go.getarcane.app/sys/crypto

go 1.27

require go.getarcane.app/acfs v0.6.2

replace (
	go.getarcane.app/acfs => ../../acfs
	go.getarcane.app/builds => ../../builds
	go.getarcane.app/docker => ../../docker
	go.getarcane.app/docker/compat => ../../docker/compat
	go.getarcane.app/docker/convert => ../../docker/convert
	go.getarcane.app/kit => ../../kit
	go.getarcane.app/streams => ../../streams
	go.getarcane.app/sys/bytes => ../bytes
	go.getarcane.app/sys/cgroup => ../cgroup
	go.getarcane.app/updater => ../../updater
)
