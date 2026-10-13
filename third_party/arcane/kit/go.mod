// Source: getarcaneapp/kit/go.mod
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relative local helper replacements

module go.getarcane.app/kit

go 1.27

require (
	github.com/jinzhu/copier v0.4.0
	golang.org/x/text v0.42.0
)

replace (
	go.getarcane.app/acfs => ../acfs
	go.getarcane.app/builds => ../builds
	go.getarcane.app/docker => ../docker
	go.getarcane.app/docker/compat => ../docker/compat
	go.getarcane.app/docker/convert => ../docker/convert
	go.getarcane.app/streams => ../streams
	go.getarcane.app/sys/bytes => ../sys/bytes
	go.getarcane.app/sys/cgroup => ../sys/cgroup
	go.getarcane.app/sys/crypto => ../sys/crypto
	go.getarcane.app/updater => ../updater
)
