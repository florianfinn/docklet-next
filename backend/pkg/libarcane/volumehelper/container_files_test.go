// Source: getarcaneapp/arcane/backend/pkg/libarcane/volumehelper/container_files_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package volumehelper

import (
	"archive/tar"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDownloadHeaderInternalRejectsDirectories(t *testing.T) {
	require.NoError(t, validateDownloadHeaderInternal(&tar.Header{Typeflag: tar.TypeReg, Mode: 0o644}))
	require.EqualError(t, validateDownloadHeaderInternal(&tar.Header{Typeflag: tar.TypeDir, Mode: 0o755}), "path is a directory")
	require.NoError(t, validateDownloadHeaderInternal(&tar.Header{Typeflag: tar.TypeSymlink, Mode: 0o777}))
	require.NoError(t, validateDownloadHeaderInternal(&tar.Header{Typeflag: tar.TypeFifo, Mode: 0o644}))
}
