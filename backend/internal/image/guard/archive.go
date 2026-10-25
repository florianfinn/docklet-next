// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package guard

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"io"
	"os"
	"path"
	"strings"
)

// ValidateArchive spools bounded input before Docker can interpret any tag.
// Docker-save manifests and the combined OCI layout emitted by current Engines
// are understood. Unknown reference-bearing metadata fails closed.
func ValidateArchive(input io.Reader, limit int64) (*os.File, func(), error) {
	fail := func() error { return imageDTO.Failure("image.invalid-archive", 400) }
	if limit <= 0 {
		return nil, nil, fail()
	}
	f, err := os.CreateTemp("", "docklet-image-*.tar")
	if err != nil {
		return nil, nil, fail()
	}
	cleanup := func() { _ = f.Close(); _ = os.Remove(f.Name()) }
	reader := bufio.NewReader(input)
	var source io.Reader = reader
	magic, _ := reader.Peek(2)
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		z, e := gzip.NewReader(reader)
		if e != nil {
			cleanup()
			return nil, nil, fail()
		}
		defer z.Close()
		source = z
	}
	n, err := io.Copy(f, io.LimitReader(source, limit+1))
	if err != nil || n > limit {
		cleanup()
		return nil, nil, fail()
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fail()
	}
	tr := tar.NewReader(f)
	found := false
	entriesByName := map[string]archiveEntry{}
	var index []byte
	layout := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			cleanup()
			return nil, nil, fail()
		}
		name := strings.TrimPrefix(h.Name, "./")
		if _, duplicate := entriesByName[name]; duplicate {
			cleanup()
			return nil, nil, fail()
		}
		offset, _ := f.Seek(0, io.SeekCurrent)
		entriesByName[name] = archiveEntry{offset, h.Size}
		if strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || h.Typeflag == tar.TypeSymlink || h.Typeflag == tar.TypeLink {
			cleanup()
			return nil, nil, fail()
		}
		if name == "index.json" || name == "oci-layout" || name == "repositories" {
			if h.Size > 4<<20 {
				cleanup()
				return nil, nil, fail()
			}
			data, e := io.ReadAll(tr)
			if e != nil {
				cleanup()
				return nil, nil, fail()
			}
			switch name {
			case "index.json":
				index = data
			case "oci-layout":
				var value struct {
					Version string `json:"imageLayoutVersion"`
				}
				if strictArchiveJSON(data, &value) != nil || value.Version != "1.0.0" {
					cleanup()
					return nil, nil, fail()
				}
				layout = true
			case "repositories":
				var repos map[string]map[string]string
				if strictArchiveJSON(data, &repos) != nil {
					cleanup()
					return nil, nil, fail()
				}
				for repo, tags := range repos {
					for tag := range tags {
						if e = ValidateReference(repo + ":" + tag); e != nil {
							cleanup()
							return nil, nil, e
						}
					}
				}
			}
			continue
		}
		if name != "manifest.json" {
			continue
		}
		if found || h.Size > 4<<20 {
			cleanup()
			return nil, nil, fail()
		}
		found = true
		var entries []struct {
			Config   string
			RepoTags []string
			Layers   []string
		}
		if e = json.NewDecoder(io.LimitReader(tr, 4<<20)).Decode(&entries); e != nil || len(entries) == 0 {
			cleanup()
			return nil, nil, fail()
		}
		for _, entry := range entries {
			if entry.Config == "" {
				cleanup()
				return nil, nil, fail()
			}
			for _, tag := range entry.RepoTags {
				if e = ValidateReference(tag); e != nil {
					cleanup()
					return nil, nil, e
				}
			}
		}
	}
	// Refuse concatenated archives or hidden declarations after the tar terminator.
	var padding [4096]byte
	for {
		n, e := f.Read(padding[:])
		for _, value := range padding[:n] {
			if value != 0 {
				cleanup()
				return nil, nil, fail()
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			cleanup()
			return nil, nil, fail()
		}
	}
	if (index != nil) != layout {
		cleanup()
		return nil, nil, fail()
	}
	if index != nil {
		if e := validateOCIArchive(f, entriesByName, index); e != nil {
			cleanup()
			return nil, nil, e
		}
	}
	if !found {
		cleanup()
		return nil, nil, fail()
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fail()
	}
	return f, cleanup, nil
}
