// Source: getarcaneapp/arcane/backend/pkg/libarcane/startup/runtime_identity_int_32bit.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build 386 || arm || mips || mipsle || wasm

package startup

import (
	"fmt"
	"strconv"
)

func parseRuntimeIdentityValueInternal(raw, key string) (int, uint32, error) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("parse %s: %w", key, err)
	}

	if value < 0 {
		return 0, 0, fmt.Errorf("%s must be >= 0", key)
	}

	return int(value), uint32(value), nil
}
