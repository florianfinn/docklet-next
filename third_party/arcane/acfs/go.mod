// Source: getarcaneapp/kit/acfs/go.mod
// Revision: 3da7cfd43c4da62bc06fd384b7da5f109ce010fb
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements; functional cut: module tidy.

module go.getarcane.app/acfs

go 1.27

require go.getarcane.app/kit v0.1.0

replace (
	go.getarcane.app/builds => ../builds
	go.getarcane.app/docker => ../docker
	go.getarcane.app/docker/compat => ../docker/compat
	go.getarcane.app/docker/convert => ../docker/convert
	go.getarcane.app/kit => ../kit
	go.getarcane.app/streams => ../streams
	go.getarcane.app/sys/bytes => ../sys/bytes
	go.getarcane.app/sys/cgroup => ../sys/cgroup
	go.getarcane.app/sys/crypto => ../sys/crypto
	go.getarcane.app/updater => ../updater
)
