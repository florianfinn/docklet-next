// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Synthetic fixture matching the reviewed Engine 29 Docker-save producer.

package guard

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

// Derived from the R1 read-only `docker save busybox:1.37` probe on Engine
// 29.8.2/containerd: index.json, oci-layout, manifest.json and digest-named blobs.
// The matching exporter enables WithSkipMissing and retains unpulled platform
// descriptors: github.com/moby/moby/blob/docker-v29.8.2/daemon/containerd/image_exporter.go
// Names, config, layers and digests are regenerated from synthetic content here.
func engine29Save(t *testing.T, annotations map[string]string, nested map[string]string, compressed bool) []byte {
	t.Helper()
	blobs := map[string]string{}
	blob := func(body any) string {
		data, e := json.Marshal(body)
		require.NoError(t, e)
		sum := sha256.Sum256(data)
		digest := fmt.Sprintf("sha256:%x", sum)
		blobs["blobs/sha256/"+digest[7:]] = string(data)
		return digest
	}
	config := blob(map[string]any{"architecture": "amd64", "os": "linux", "rootfs": map[string]any{"type": "layers", "diff_ids": []string{}}})
	manifest := blob(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": map[string]any{"mediaType": "application/vnd.oci.image.config.v1+json", "digest": config, "size": len(blobs["blobs/sha256/"+config[7:]])}, "layers": []any{}})
	index := blob(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": []any{map[string]any{"mediaType": "application/vnd.oci.image.manifest.v1+json", "digest": manifest, "size": len(blobs["blobs/sha256/"+manifest[7:]]), "platform": map[string]string{"architecture": "amd64", "os": "linux"}, "annotations": nested}, map[string]any{"mediaType": "application/vnd.oci.image.manifest.v1+json", "digest": fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("synthetic unpulled platform"))), "size": 100, "platform": map[string]string{"architecture": "arm64", "os": "linux"}}}})
	top, e := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": []any{map[string]any{"mediaType": "application/vnd.oci.image.index.v1+json", "digest": index, "size": len(blobs["blobs/sha256/"+index[7:]]), "annotations": annotations}}})
	require.NoError(t, e)
	blobs["index.json"] = string(top)
	blobs["oci-layout"] = `{"imageLayoutVersion":"1.0.0"}`
	body := archiveBytes(t, []map[string]any{{"Config": "blobs/sha256/" + config[7:], "RepoTags": []string{"registry.example.com/app:1.0.0"}, "Layers": []string{}}}, blobs)
	if compressed {
		var zipped bytes.Buffer
		writer := gzip.NewWriter(&zipped)
		_, e = writer.Write(body)
		require.NoError(t, e)
		require.NoError(t, writer.Close())
		return zipped.Bytes()
	}
	return body
}

func TestEngine29DockerSaveLayoutAndReferenceAnnotations(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprint(compressed), func(t *testing.T) {
			good := map[string]string{"io.containerd.image.name": "registry.example.com/app:1.0.0", "org.opencontainers.image.ref.name": "1.0.0"}
			body := engine29Save(t, good, nil, compressed)
			f, cleanup, err := ValidateArchive(bytes.NewReader(body), 1<<20)
			require.NoError(t, err)
			defer cleanup()
			_, err = io.Copy(io.Discard, f)
			require.NoError(t, err)
			for _, key := range []string{"io.containerd.image.name", "org.opencontainers.image.ref.name"} {
				t.Run(key, func(t *testing.T) {
					bad := map[string]string{}
					for k, v := range good {
						bad[k] = v
					}
					bad[key] = "docklet-retention.invalid/points/p/s:saved"
					if key == "io.containerd.image.name" {
						delete(bad, "org.opencontainers.image.ref.name")
					}
					_, _, err := ValidateArchive(bytes.NewReader(engine29Save(t, bad, nil, compressed)), 1<<20)
					require.Equal(t, "image.reserved-reference", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
				})
			}
		})
	}
}
func TestEngine29ArchiveIgnoresNonCarrierAnnotations(t *testing.T) {
	for _, test := range []struct{ top, nested map[string]string }{
		{map[string]string{"example.image.name": "registry.example.com/app:1.0.0"}, nil},
		{nil, map[string]string{"io.containerd.image.name": "docklet-retention.invalid/points/p/s:saved"}},
		{map[string]string{"org.opencontainers.image.base.name": "docklet-retention.invalid/points/p/s:saved"}, nil},
	} {
		_, cleanup, err := ValidateArchive(bytes.NewReader(engine29Save(t, test.top, test.nested, false)), 1<<20)
		require.NoError(t, err)
		cleanup()
	}
}

func TestEngine29ArchiveRequiresImageNameForTagOnlyCarrier(t *testing.T) {
	_, _, err := ValidateArchive(bytes.NewReader(engine29Save(t, map[string]string{"org.opencontainers.image.ref.name": "1.0.0"}, nil, false)), 1<<20)
	require.Error(t, err)
}

func TestEngine29ArchiveRequiresTopLevelContent(t *testing.T) {
	body := engine29Save(t, map[string]string{"io.containerd.image.name": "registry.example.com/app:1.0.0"}, nil, false)
	files := map[string]string{}
	reader := tar.NewReader(bytes.NewReader(body))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		files[header.Name] = string(data)
	}
	var index struct {
		Manifests []archiveDescriptor `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal([]byte(files["index.json"]), &index))
	delete(files, "blobs/sha256/"+index.Manifests[0].Digest[7:])
	_, _, err := ValidateArchive(bytes.NewReader(archiveBytes(t, nil, files)), 1<<20)
	require.Equal(t, "image.invalid-archive", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
}
