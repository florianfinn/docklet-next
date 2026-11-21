// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original peer name evidence for unreadable stacks (issue 547).

package projects

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

const peerEvidenceLimit = 1 << 20

type peerNamesHiddenKey struct{}

// WithPeerNamesHidden marks a caller restricted to assigned stacks: other
// stacks' names must not appear in that caller's responses.
func WithPeerNamesHidden(ctx context.Context) context.Context {
	return context.WithValue(ctx, peerNamesHiddenKey{}, true)
}
func PeerNamesHidden(ctx context.Context) bool { return ctx.Value(peerNamesHiddenKey{}) != nil }

var overrideFileNames = []string{"compose.override.yaml", "compose.override.yml", "docker-compose.override.yaml", "docker-compose.override.yml"}

// PeerLiteralName is warning-only evidence for a stack whose model cannot be
// loaded: the plain, dollar-free top-level name of its single conventional
// Compose file. Any override, selection variable or unreadable input yields "".
// It never authorizes a runtime target.
func PeerLiteralName(dir string) string {
	if env, err := readBounded(filepath.Join(dir, ".env")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ""
	} else if envSelects(env) {
		return ""
	}
	for _, name := range overrideFileNames {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			return ""
		}
	}
	var file string
	for _, candidate := range composeFileCandidates {
		info, err := os.Stat(filepath.Join(dir, candidate))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || info.IsDir() || file != "" {
			return ""
		}
		file = filepath.Join(dir, candidate)
	}
	if file == "" {
		return ""
	}
	content, err := readBounded(file)
	if err != nil {
		return ""
	}
	return literalName(content)
}

func envSelects(content []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "export "))
		for _, key := range []string{"COMPOSE_FILE", "COMPOSE_PROJECT_NAME", "COMPOSE_PATH_SEPARATOR", "COMPOSE_ENV_FILES"} {
			if strings.HasPrefix(line, key+"=") || strings.HasPrefix(line, key+" ") {
				return true
			}
		}
	}
	return scanner.Err() != nil
}

func literalName(content []byte) string {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var doc yaml.Node
	if decoder.Decode(&doc) != nil {
		return ""
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ""
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return ""
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Value != "name" {
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Style != 0 || value.Tag != "!!str" || value.Anchor != "" || strings.Contains(value.Value, "$") {
			return ""
		}
		return NormalizeProjectName(value.Value)
	}
	return ""
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, peerEvidenceLimit+1))
	if err != nil {
		return nil, err
	}
	if len(content) > peerEvidenceLimit {
		return nil, errors.New("peer evidence input too large")
	}
	return content, nil
}
