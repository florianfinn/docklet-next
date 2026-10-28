// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original test for the process exit status of a fatal shutdown.

package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeSeparatesFatalFromCleanShutdown(t *testing.T) {
	if code := exitCode(nil); code != 0 {
		t.Errorf("clean shutdown exit code = %d, want 0", code)
	}
	fatal := fmt.Errorf("%w: %w", errors.New("actor host lost"), errors.New("host is not registered"))
	if code := exitCode(fatal); code != 1 {
		t.Errorf("fatal shutdown exit code = %d, want 1", code)
	}
}
