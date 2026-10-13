// Source: getarcaneapp/arcane/backend/pkg/utils/httpx/socket_linux.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build linux

package httpx

import (
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func setDeadPeerTimeoutInternal(rawConn syscall.RawConn, timeout time.Duration) error {
	var setErr error
	if err := rawConn.Control(func(fd uintptr) {
		setErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, int(timeout.Milliseconds()))
	}); err != nil {
		return err
	}
	return setErr
}
