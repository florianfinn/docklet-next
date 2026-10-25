// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Current Docker-save OCI metadata preflight for issue #229.

package guard

import (
	"bytes"
	"encoding/json"
	"github.com/distribution/reference"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"io"
	"os"
	"strings"
)

type archiveEntry struct{ offset, size int64 }
type archiveDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Annotations map[string]string `json:"annotations"`
}

func strictArchiveJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return imageDTO.Failure("image.invalid-archive", 400)
	}
	return nil
}

func validateOCIArchive(file *os.File, entries map[string]archiveEntry, index []byte) error {
	invalid := func() error { return imageDTO.Failure("image.invalid-archive", 400) }
	// Docker imports names only from the exact top-level descriptor carriers.
	// Nested annotations describe provenance and do not assign imported tags.
	var root struct {
		SchemaVersion int                 `json:"schemaVersion"`
		Manifests     []archiveDescriptor `json:"manifests"`
		Annotations   map[string]string   `json:"annotations"`
	}
	if strictArchiveJSON(index, &root) != nil || root.SchemaVersion != 2 || len(root.Manifests) == 0 {
		return invalid()
	}
	seen := map[string]bool{}
	var walk func(archiveDescriptor, int) error
	walk = func(d archiveDescriptor, depth int) error {
		if depth > 16 || len(seen) > 4096 {
			return invalid()
		}
		if !strings.HasPrefix(d.Digest, "sha256:") || len(d.Digest) != 71 || strings.Trim(d.Digest[7:], "0123456789abcdef") != "" {
			return invalid()
		}
		switch d.MediaType {
		case "application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json":
		default:
			return invalid()
		}
		if seen[d.Digest] {
			return nil
		}
		seen[d.Digest] = true
		entry, ok := entries["blobs/sha256/"+d.Digest[7:]]
		if !ok {
			// Engine exports retain the multi-platform index while skipping unpulled
			// platform manifests; nested annotations cannot assign imported names.
			if depth > 0 && (d.MediaType == "application/vnd.oci.image.manifest.v1+json" || d.MediaType == "application/vnd.docker.distribution.manifest.v2+json") {
				return nil
			}
			return invalid()
		}
		if entry.size > 4<<20 {
			return invalid()
		}
		data, err := io.ReadAll(io.NewSectionReader(file, entry.offset, entry.size))
		if err != nil {
			return invalid()
		}
		var metadata map[string]any
		if strictArchiveJSON(data, &metadata) != nil {
			return invalid()
		}
		if strings.Contains(d.MediaType, "index") || strings.Contains(d.MediaType, "list") {
			var nested struct {
				SchemaVersion int                 `json:"schemaVersion"`
				Manifests     []archiveDescriptor `json:"manifests"`
			}
			if strictArchiveJSON(data, &nested) != nil || nested.SchemaVersion != 2 || len(nested.Manifests) == 0 {
				return invalid()
			}
			for _, child := range nested.Manifests {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, d := range root.Manifests {
		imageName := d.Annotations["io.containerd.image.name"]
		refName := d.Annotations["org.opencontainers.image.ref.name"]
		if imageName != "" {
			if err := ValidateReference(imageName); err != nil {
				return err
			}
		}
		if refName != "" {
			candidate := refName
			if !strings.ContainsAny(refName, "/:") {
				named, err := reference.ParseNormalizedNamed(imageName)
				if err != nil {
					return invalid()
				}
				candidate = reference.TrimNamed(named).String() + ":" + refName
			}
			if err := ValidateReference(candidate); err != nil {
				return err
			}
		}
		if err := walk(d, 0); err != nil {
			return err
		}
	}
	return nil
}
