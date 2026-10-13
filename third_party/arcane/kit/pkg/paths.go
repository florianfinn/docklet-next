// Source: getarcaneapp/kit/pkg/paths.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed SQLite runtime and migrations.

package kit

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// NormalizeRelativePath validates and cleans a slash-delimited path rooted at a
// managed file tree. The result never begins with a slash or escapes the root.
func NormalizeRelativePath(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	switch {
	case trimmed == "":
		return "", errors.New("path is required")
	case strings.ContainsRune(trimmed, 0):
		return "", errors.New("path contains a null byte")
	case strings.Contains(trimmed, `\`):
		return "", errors.New("path must use forward slashes")
	case path.IsAbs(trimmed) || filepath.IsAbs(trimmed):
		return "", errors.New("absolute paths are not allowed")
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("path traversal is not allowed")
	}
	return cleaned, nil
}

// ValidateFileName validates a single file-tree path segment.
func ValidateFileName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "":
		return "", errors.New("name is required")
	case strings.ContainsRune(trimmed, 0):
		return "", errors.New("name contains a null byte")
	case strings.ContainsAny(trimmed, `/\`):
		return "", errors.New("name must not contain path separators")
	case filepath.VolumeName(trimmed) != "":
		return "", errors.New("name must not contain a volume prefix")
	case trimmed == "." || trimmed == "..":
		return "", errors.New("invalid name")
	}
	return trimmed, nil
}

// FilePathMatches reports whether relativePath is rootPath or lies beneath it.
func FilePathMatches(relativePath, rootPath string) bool {
	return relativePath == rootPath || strings.HasPrefix(relativePath, rootPath+"/")
}

// SanitizeBrowsePath cleans a path within a rooted file browser to an absolute
// form, rejecting traversal above the root.
func SanitizeBrowsePath(input string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(input))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("invalid path: path traversal not allowed")
	}
	return path.Clean("/" + cleaned), nil
}
