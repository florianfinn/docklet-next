// Source: getarcaneapp/arcane/backend/pkg/projects/cache.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global. M34 #548: recorded Compose environment inputs.

package projects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/samber/hot"
)

type composeCacheInternal[T any] struct {
	entries *hot.HotCache[string, composeCacheEntryInternal[T]]
	clone   func(T) T
}

type composeCacheEntryInternal[T any] struct {
	fingerprint string
	mtimes      map[string]time.Time
	value       T
}

// NewComposeCache caches Compose-derived values and their file dependencies.
// clone, when supplied, isolates stored values from mutations by callers.
func NewComposeCache[T any](capacity int, clone func(T) T) projecttypes.ComposeCache[T] {
	return &composeCacheInternal[T]{entries: hot.NewHotCache[string, composeCacheEntryInternal[T]](hot.LRU, capacity).Build(), clone: clone}
}

// NewParsedComposeCache keeps operation-local models separate from cached state.
func NewParsedComposeCache() projecttypes.ComposeCache[*composetypes.Project] {
	return NewComposeCache(2048, cloneComposeProjectInternal)
}

func cloneComposeProjectInternal(project *composetypes.Project) *composetypes.Project {
	if project == nil {
		return nil
	}
	// With no names this compose-go operation deep-copies without disabling services.
	return project.WithServicesDisabled()
}

func (c *composeCacheInternal[T]) Get(projectID, fingerprint string) (T, bool) {
	entry, found := c.entries.Peek(projectID)
	if found && entry.fingerprint == fingerprint && validComposeCacheEntryInternal(entry.mtimes) {
		if c.clone != nil {
			return c.clone(entry.value), true
		}
		return entry.value, true
	}
	var zero T
	return zero, false
}

func (c *composeCacheInternal[T]) Set(projectID, fingerprint, projectPath, projectsDirectory, composePath string, composeFiles, envFiles []string, value T) error {
	if strings.TrimSpace(projectID) == "" {
		return nil
	}
	mtimes := make(map[string]time.Time)
	for _, path := range append([]string{composePath}, composeFiles...) {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat compose file %s: %w", path, err)
		}
		mtimes[path] = info.ModTime()
	}
	dependencies := slices.Clone(envFiles)
	if !strings.HasPrefix(fingerprint, "recorded|") {
		dependencies = append(dependencies, filepath.Join(projectPath, EffectiveEnvFileName))
	}
	// A later standard override must invalidate a model loaded before it existed.
	for _, name := range composeOverrideFileCandidates {
		dependencies = append(dependencies, filepath.Join(filepath.Dir(composePath), name))
	}
	for _, path := range dependencies {
		info, err := os.Stat(path)
		if err == nil {
			mtimes[path] = info.ModTime()
			continue
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat compose dependency %s: %w", path, err)
		}
		mtimes[path] = time.Time{}
	}
	if c.clone != nil {
		value = c.clone(value)
	}
	c.entries.Set(projectID, composeCacheEntryInternal[T]{fingerprint: fingerprint, mtimes: mtimes, value: value})
	return nil
}

func (c *composeCacheInternal[T]) Invalidate(projectID string) {
	c.entries.Delete(projectID)
}

func validComposeCacheEntryInternal(mtimes map[string]time.Time) bool {
	for path, mtime := range mtimes {
		info, err := os.Stat(path)
		if err != nil {
			if !os.IsNotExist(err) || !mtime.IsZero() {
				return false
			}
			continue
		}
		if mtime.IsZero() || !info.ModTime().Equal(mtime) {
			return false
		}
		// A chmod does not bump mtime, so confirm the file is still readable.
		file, err := os.Open(path)
		if err != nil {
			return false
		}
		if closeErr := file.Close(); closeErr != nil {
			return false
		}
	}
	return true
}

// LoadCachedComposeProject resolves an executable model with isolated cached state.
func LoadCachedComposeProject(ctx context.Context,
	cache projecttypes.ComposeCache[*composetypes.Project],
	projectID,
	projectPath,
	composePath,
	projectName,
	projectsDirectory string,
	autoInject bool,
	pathMapper *PathMapper) (*composetypes.Project,
	error,
) {
	input, err := ResolveComposeInputs(ctx, projectsDirectory, projectPath, autoInject)
	if err != nil {
		return nil, err
	}
	ctx = WithResolvedInputs(ctx, input)
	fingerprint := input.State + "|" + fmt.Sprintf("%q|%q|%q|%t|%#v|%s", composePath, projectName, projectsDirectory, autoInject, pathMapper, input.DependencyHash)
	if cache != nil {
		if cached, ok := cache.Get(projectID, fingerprint); ok {
			return cached, nil
		}
	}
	dependencies := projecttypes.ComposeDependencies{}
	model, err := LoadComposeProject(ctx, composePath, projectName, projectsDirectory, autoInject, pathMapper, nil, nil, false, &dependencies, nil, nil)
	if err != nil {
		return nil, err
	}
	if cache == nil {
		return model, nil
	}
	if envOpts, parseErr := ParseComposeEnvOptions(model.WorkingDir, EnvMap(model.Environment)); parseErr == nil {
		dependencies.EnvFiles = append(dependencies.EnvFiles, envOpts.EnvFiles...)
	}
	// Metadata discovery already follows recursive includes and their interpolation env files.
	meta, err := ParseArcaneComposeMetadata(ctx, composePath, projectsDirectory, autoInject)
	if err != nil {
		return model, nil //nolint:nilerr // Metadata discovery failure disables caching; the executable model is valid.
	}
	composeFiles := append(append([]string{}, model.ComposeFiles...), meta.ComposeFiles...)
	dependencies.EnvFiles = append(dependencies.EnvFiles, meta.EnvFiles...)
	if input.Recorded() {
		dependencies.EnvFiles = slices.DeleteFunc(dependencies.EnvFiles, func(path string) bool { return slices.Contains(input.Paths, path) })
	}
	if setErr := cache.Set(projectID, fingerprint, projectPath, projectsDirectory, composePath, composeFiles, dependencies.EnvFiles, model); setErr != nil {
		return nil, setErr
	}
	return model, nil
}
