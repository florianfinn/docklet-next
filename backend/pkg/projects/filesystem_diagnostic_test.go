// Source: docklet-next/issues/674
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package projects

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"log/slog"
	"os"
	"testing"
)

func TestFilesystemPermissionDiagnosticIsNeutral(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	err := FilesystemError(t.Context(), "create project directory", "/projects/example", os.ErrPermission)
	require.ErrorIs(t, err, os.ErrPermission)
	require.Equal(t, "project filesystem operation failed", err.Error())
	require.Contains(t, output.String(), `"path":"/projects/example"`)
	require.Contains(t, output.String(), fmt.Sprintf(`"uid":%d`, os.Getuid()))
	require.Contains(t, output.String(), fmt.Sprintf(`"gid":%d`, os.Getgid()))
	require.False(t, errors.Is(err, os.ErrExist))
}
