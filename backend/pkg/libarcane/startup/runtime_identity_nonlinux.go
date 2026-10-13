// Source: getarcaneapp/arcane/backend/pkg/libarcane/startup/runtime_identity_nonlinux.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build !linux

package startup

import (
	"context"
	"fmt"
	"runtime"
)

var (
	_ = runtimeIdentitySupplementaryGroupsInternal
	_ = dockerSocketPathInternal
)

func reexecWithRuntimeIdentityInternal(_ context.Context, req runtimeIdentityRequest) error {
	return fmt.Errorf("runtime identity switching is only supported on linux, current platform is %s", runtime.GOOS)
}
