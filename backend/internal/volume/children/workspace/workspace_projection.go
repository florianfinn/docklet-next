// Source: getarcaneapp/arcane/backend/internal/volume/children/workspace/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; mutation and rollback helpers extracted to mutations.go; M12 persisted volume lifecycle and legacy restore cut; M16 bind mounts and backup completion.

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	acfsutils "github.com/florianfinn/docklet-next/backend/pkg/utils/acfs"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
	"github.com/florianfinn/docklet-next/types/workspace"
	"go.getarcane.app/acfs/types"
	"go.getarcane.app/kit/pkg"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
)

func volumeWorkspaceFileContentResponseInternal(relativePath, kind string, size int64, content []byte, maxFileSizeBytes int64) (*workspace.FileContent, error) {
	response := &workspace.FileContent{
		Path:         "/" + relativePath,
		RelativePath: relativePath,
		Name:         path.Base(relativePath),
		MimeType:     http.DetectContentType(content),
		Size:         size,
	}
	if kind == "special" {
		response.ReadOnlyReason = workspace.FileReadOnlySpecial
		return response, nil
	}
	if kind != "regular" {
		return nil, errors.New("invalid volume workspace file kind")
	}
	if size > maxFileSizeBytes {
		response.ReadOnlyReason = workspace.FileReadOnlyTooLarge
		return response, nil
	}
	if !workspacepkg.IsTextContent(content) {
		response.ReadOnlyReason = workspace.FileReadOnlyBinary
		return response, nil
	}
	response.Editable = true
	response.Content = string(content)
	return response, nil
}

func volumeWorkspaceEntryFromACFSInternal(entry types.Entry, maxFileSizeBytes int64) (workspace.FileEntry, string) {
	fileEntry := acfsutils.FileEntry(entry)
	fileEntry.RelativePath = strings.TrimPrefix(entry.Path, "/")
	switch {
	case entry.IsDirectory:
		fileEntry.Size = 0
		fileEntry.Editable = true
		return fileEntry, "dir"
	case entry.IsSymlink:
		fileEntry.ReadOnlyReason = workspace.FileReadOnlySymlink
		return fileEntry, "symlink"
	case acfsutils.IsRegular(entry):
		if entry.Size > maxFileSizeBytes {
			fileEntry.ReadOnlyReason = workspace.FileReadOnlyTooLarge
		} else {
			fileEntry.Editable = true
		}
		return fileEntry, "file"
	default:
		fileEntry.ReadOnlyReason = workspace.FileReadOnlySpecial
		return fileEntry, "special"
	}
}

func decodeVolumeWorkspaceWalkInternal(source io.Reader, maxEntries int, maxFileSizeBytes int64) (*workspace.Workspace, error) {
	decoder := jsontext.NewDecoder(source)
	files := make([]workspace.FileEntry, 0, min(maxEntries, 256))
	classifications := make(map[string]string, min(maxEntries, 256))
	trailerSeen := false
	truncated := false
	for {
		var record types.WalkRecord
		err := json.UnmarshalDecode(decoder, &record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if record.Version != types.ProtocolVersion {
			return nil, fmt.Errorf("unsupported acfs protocol %d", record.Version)
		}
		if trailerSeen {
			return nil, errors.New("acfs walk emitted a record after its trailer")
		}
		if record.End {
			trailerSeen = true
			truncated = record.Truncated
			if record.Count != len(files) {
				return nil, errors.New("acfs walk trailer count does not match emitted entries")
			}
			continue
		}
		if record.Entry == nil {
			return nil, errors.New("acfs walk emitted an empty record")
		}
		if len(files) >= maxEntries {
			return nil, errors.New("acfs walk exceeded the requested entry limit")
		}
		fileEntry, classification := volumeWorkspaceEntryFromACFSInternal(*record.Entry, maxFileSizeBytes)
		classifications[fileEntry.RelativePath] = classification
		files = append(files, fileEntry)
	}
	if !trailerSeen {
		return nil, errors.New("acfs walk ended without a trailer")
	}

	revisionEntries := slices.Clone(files)
	slices.SortFunc(revisionEntries, func(a, b workspace.FileEntry) int {
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	h := sha256.New()
	for _, entry := range revisionEntries {
		kit.WriteRecord(h, entry.RelativePath, classifications[entry.RelativePath], entry.Size, entry.ModTime.UnixNano(), entry.Mode)
	}
	slices.SortFunc(files, func(a, b workspace.FileEntry) int {
		if a.IsDirectory != b.IsDirectory {
			return kit.Ternary(a.IsDirectory, -1, 1)
		}
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	return &workspace.Workspace{
		Files:             files,
		FileTreeRevision:  hex.EncodeToString(h.Sum(nil)),
		FileTreeTruncated: truncated,
	}, nil
}
