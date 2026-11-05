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
	"fmt"
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

// Each unsafe member is refused on its own with the typed archive code.
func TestArchiveRefusesUnsafeMembers(t *testing.T) {
	manifest, err := json.Marshal([]map[string]any{{"Config": "config.json", "RepoTags": []string{"registry.example.com/app:1.0.0"}, "Layers": []string{}}})
	require.NoError(t, err)
	for _, tc := range []string{"dot-dot", "char-device", "block-device", "fifo", "too-many-entries", "valid"} {
		t.Run(tc, func(t *testing.T) {
			var b bytes.Buffer
			w := tar.NewWriter(&b)
			require.NoError(t, w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}))
			_, err := w.Write(manifest)
			require.NoError(t, err)
			require.NoError(t, w.WriteHeader(&tar.Header{Name: "layer/", Mode: 0700, Typeflag: tar.TypeDir}))
			switch tc {
			case "dot-dot":
				require.NoError(t, w.WriteHeader(&tar.Header{Name: "..", Mode: 0700, Typeflag: tar.TypeDir}))
			case "char-device":
				require.NoError(t, w.WriteHeader(&tar.Header{Name: "layer/null", Mode: 0600, Typeflag: tar.TypeChar}))
			case "block-device":
				require.NoError(t, w.WriteHeader(&tar.Header{Name: "layer/disk", Mode: 0600, Typeflag: tar.TypeBlock}))
			case "fifo":
				require.NoError(t, w.WriteHeader(&tar.Header{Name: "layer/pipe", Mode: 0600, Typeflag: tar.TypeFifo}))
			case "too-many-entries":
				for i := 0; i < maxArchiveEntries; i++ {
					require.NoError(t, w.WriteHeader(&tar.Header{Name: fmt.Sprintf("layer/%d", i), Mode: 0600, Typeflag: tar.TypeReg}))
				}
			}
			require.NoError(t, w.Close())
			_, cleanup, err := ValidateArchive(bytes.NewReader(b.Bytes()), 64<<20)
			if tc == "valid" {
				require.NoError(t, err)
				cleanup()
				return
			}
			require.Equal(t, "image.invalid-archive", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
		})
	}
}
