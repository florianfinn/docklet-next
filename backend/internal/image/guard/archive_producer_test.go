// Source: docklet-next/docs/design/images-updates.md
// Revision: 474f31a45f4909c153dee30a37306fcd838678da
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Preserve real JSON metadata, replace binary layers with dummy bytes.

package guard

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

var producerArchives = []struct {
	name, fixture string
	gzip          bool
}{
	{"busybox", "busybox", false},
	{"alpine", "alpine", false},
	{"alpine-gzip", "alpine", true},
	{"nginx", "nginx", false},
	{"local", "local", false},
}

func producerFiles(t *testing.T, name string) map[string]string {
	t.Helper()
	root := filepath.Join("testdata", "docker-save", name)
	data, err := os.ReadFile(filepath.Join(root, "members.json"))
	require.NoError(t, err)
	var members []struct {
		Path string
		JSON bool
	}
	require.NoError(t, json.Unmarshal(data, &members))
	files := map[string]string{}
	for _, member := range members {
		body := []byte("dummy layer bytes")
		if member.JSON {
			body, err = os.ReadFile(filepath.Join(root, member.Path))
			require.NoError(t, err)
			require.True(t, json.Valid(body))
			if strings.HasPrefix(member.Path, "blobs/sha256/") {
				require.Equal(t, strings.TrimPrefix(member.Path, "blobs/sha256/"), fmt.Sprintf("%x", sha256.Sum256(body)))
			}
		}
		files[member.Path] = string(body)
	}
	return files
}

func producerArchive(t *testing.T, files map[string]string, compressed bool) []byte {
	t.Helper()
	body := archiveBytes(t, nil, files)
	if !compressed {
		return body
	}
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	_, err := writer.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return out.Bytes()
}

func TestDockerSaveProducerArchives(t *testing.T) {
	for _, test := range producerArchives {
		t.Run(test.name, func(t *testing.T) {
			files := producerFiles(t, test.fixture)
			_, cleanup, err := ValidateArchive(bytes.NewReader(producerArchive(t, files, test.gzip)), 1<<20)
			require.NoError(t, err)
			cleanup()
		})
	}
}

func TestDockerSaveProducerReservedReferenceCarriers(t *testing.T) {
	for _, test := range producerArchives {
		t.Run(test.name, func(t *testing.T) {
			for _, carrier := range []string{"io.containerd.image.name", "org.opencontainers.image.ref.name", "RepoTags"} {
				t.Run(carrier, func(t *testing.T) {
					files := producerFiles(t, test.fixture)
					reserved := "docklet-retention.invalid/points/example/image:saved"
					if carrier == "RepoTags" {
						var manifest []map[string]any
						require.NoError(t, json.Unmarshal([]byte(files["manifest.json"]), &manifest))
						manifest[0]["RepoTags"] = []string{reserved}
						data, err := json.Marshal(manifest)
						require.NoError(t, err)
						files["manifest.json"] = string(data)
					} else {
						var index struct {
							SchemaVersion int                 `json:"schemaVersion"`
							Manifests     []archiveDescriptor `json:"manifests"`
						}
						require.NoError(t, json.Unmarshal([]byte(files["index.json"]), &index))
						index.Manifests[0].Annotations[carrier] = reserved
						if carrier == "io.containerd.image.name" {
							delete(index.Manifests[0].Annotations, "org.opencontainers.image.ref.name")
						}
						data, err := json.Marshal(index)
						require.NoError(t, err)
						files["index.json"] = string(data)
					}
					_, _, err := ValidateArchive(bytes.NewReader(producerArchive(t, files, test.gzip)), 1<<20)
					require.Equal(t, "image.reserved-reference", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
				})
			}
		})
	}
}
