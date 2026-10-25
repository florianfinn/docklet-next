// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package guard

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"testing"
)

func archiveBytes(t *testing.T, manifest any, extra map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	if manifest != nil {
		body, err := json.Marshal(manifest)
		require.NoError(t, err)
		extra["manifest.json"] = string(body)
	}
	for name, body := range extra {
		require.NoError(t, w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(body))}))
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return b.Bytes()
}
func TestArchiveRejectsEveryReservedTagBeforeLoad(t *testing.T) {
	for _, ref := range []string{"docklet-retention.invalid/points/a/b:saved", "DOCKLET-RETENTION.INVALID/runs/a/b:frozen"} {
		t.Run(ref, func(t *testing.T) {
			body := archiveBytes(t, []map[string]any{{"Config": "config.json", "RepoTags": []string{"registry.example.com/app:1.0.0", ref}, "Layers": []string{}}}, map[string]string{})
			_, _, err := ValidateArchive(bytes.NewReader(body), 1<<20)
			require.Equal(t, "image.reserved-reference", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
		})
	}
}
func TestArchiveValidatesLegacyRepositoriesAndRefusesUnknownOCI(t *testing.T) {
	for _, extra := range []map[string]string{{"repositories": `{"docklet-retention.invalid/points/a/b":{"saved":"image"}}`}, {"index.json": `{"manifests":[]}`}} {
		body := archiveBytes(t, []map[string]any{{"Config": "config.json", "RepoTags": []string{"registry.example.com/app:1.0.0"}, "Layers": []string{}}}, extra)
		_, _, err := ValidateArchive(bytes.NewReader(body), 1<<20)
		require.Error(t, err)
	}
}
func TestArchiveBoundAndValidatedReplay(t *testing.T) {
	body := archiveBytes(t, []map[string]any{{"Config": "config.json", "RepoTags": []string{"registry.example.com/app:1.0.0"}, "Layers": []string{}}}, map[string]string{})
	file, cleanup, err := ValidateArchive(bytes.NewReader(body), 1<<20)
	require.NoError(t, err)
	defer cleanup()
	var replay bytes.Buffer
	_, err = replay.ReadFrom(file)
	require.NoError(t, err)
	require.Equal(t, body, replay.Bytes())
	_, _, err = ValidateArchive(bytes.NewReader(body), 100)
	require.Error(t, err)
}
