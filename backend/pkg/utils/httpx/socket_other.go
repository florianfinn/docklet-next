// Source: getarcaneapp/arcane/backend/pkg/utils/httpx/socket_other.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build !linux

package httpx

import (
	"syscall"
	"time"
)

// setDeadPeerTimeoutInternal is a no-op outside Linux. TCP_USER_TIMEOUT has no
// portable equivalent, and Arcane only ships Linux images; local dev builds
// keep the OS default retransmission behaviour.
//
//nolint:shimbad // build-tag stub: the real implementation lives in socket_linux.go
func setDeadPeerTimeoutInternal(_ syscall.RawConn, _ time.Duration) error {
	return nil
}
