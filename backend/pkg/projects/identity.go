// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package projects

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

type MissingServiceEnvironmentError struct{ Cause error }

func (e *MissingServiceEnvironmentError) Error() string {
	return "required service environment is unavailable"
}
func (e *MissingServiceEnvironmentError) Unwrap() error { return e.Cause }

// LoadComposeIdentity keeps strict executable loading, allowing only verified
// missing required service env files on a disposable identity-only model.
// It performs no bind preparation or host path translation.
func LoadComposeIdentity(ctx context.Context, composeFile, projectName, projectsDirectory string, autoInject bool) (*composetypes.Project, []string, error) {
	load := func(skip bool) (*composetypes.Project, error) {
		return LoadComposeProject(ctx, composeFile, projectName, projectsDirectory, autoInject, nil, nil, func(o *loader.Options) { o.SkipResolveEnvironment = skip }, false, nil, nil, nil)
	}
	full, original := load(false)
	if original == nil {
		return full, nil, nil
	}
	var missing *fs.PathError
	if !errors.As(original, &missing) || !errors.Is(missing.Err, fs.ErrNotExist) {
		return nil, nil, original
	}
	model, err := load(true)
	if err != nil {
		return nil, nil, original
	}
	verified := false
	var absent []string
	for key, service := range model.Services {
		for i, entry := range service.EnvFiles {
			if !entry.Required {
				continue
			}
			_, statErr := os.Stat(entry.Path)
			if !errors.Is(statErr, fs.ErrNotExist) {
				continue
			}
			if filepath.Clean(entry.Path) == filepath.Clean(missing.Path) {
				verified = true
			}
			absent = append(absent, entry.Path)
			service.EnvFiles[i].Required = false
		}
		model.Services[key] = service
	}
	if !verified {
		return nil, nil, original
	}
	resolved, err := model.WithServicesEnvironmentResolved(false)
	if err != nil {
		return nil, absent, &MissingServiceEnvironmentError{Cause: err}
	}
	return resolved, absent, nil
}
