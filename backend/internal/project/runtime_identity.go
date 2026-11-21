// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Fresh, read-only project identity for runtime control. M34 #548: recorded Compose environment inputs. M35 #558: ID-only targets read their own row.

package project

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	kit "go.getarcane.app/kit/pkg"
)

func (s *ProjectService) ResolveRuntimeProjectIdentity(ctx context.Context, projectID, composeName string) (projecttypes.Reference, bool, error) {
	ref, found, _, err := s.ResolveRuntimeProjectIdentityAcknowledged(ctx, projectID, composeName, "")
	return ref, found, err
}

// ID-only actions read fresh evidence from their target alone. A supplied name
// requires the full ownership scan; unreadable peers are skipped and duplicate
// names refuse. The contract-20 acknowledgement is accepted but never needed.
func (s *ProjectService) ResolveRuntimeProjectIdentityAcknowledged(ctx context.Context, projectID, composeName, _ string) (projecttypes.Reference, bool, *imageDTO.RuntimePeerOverrides, error) {
	ctx = s.composeInputContext(ctx)
	empty := projecttypes.Reference{}
	if projectID == "" && composeName == "" {
		return empty, false, nil, nil
	}
	var rows []Project
	query := s.db.WithContext(ctx)
	if projectID != "" && composeName == "" {
		query = query.Where("id = ?", projectID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return empty, false, nil, fmt.Errorf("list runtime project identities: %w", err)
	}
	type evidence struct {
		row        *Project
		name, file string
	}
	all := make([]evidence, 0, len(rows))
	owners := map[string][]int{}
	match := -1
	for i := range rows {
		row := &rows[i]
		name, file, err := s.runtimeComposeEvidence(ctx, row)
		if err != nil {
			if row.ID == projectID {
				return empty, false, nil, fmt.Errorf("read runtime project identity: %w", err)
			}
			all = append(all, evidence{row: row})
			continue
		}
		all = append(all, evidence{row, name, file})
		if name != "" {
			owners[name] = append(owners[name], i)
			if projectID != "" && row.ID == projectID || projectID == "" && name == composeName {
				match = i
			}
		}
	}
	if match < 0 {
		return empty, false, nil, nil
	}
	current := all[match]
	// A recovered peer name is useful evidence but never authorizes that row as a target.
	if current.file != "" && projectID != "" {
		return empty, false, nil, fmt.Errorf("target service environment unavailable")
	}
	if composeName != "" && current.name != composeName {
		return empty, false, nil, nil
	}
	if composeName != "" && len(owners[current.name]) != 1 {
		peer := current
		for _, i := range owners[current.name] {
			if i != match {
				peer = all[i]
				break
			}
		}
		blocker := &imageDTO.RuntimeIdentityBlocker{Name: imageDTO.RuntimeDisplayName(peer.row.Name, peer.row.ID), PeerID: peer.row.ID, Archived: peer.row.IsArchived, Reason: "compose.duplicate", File: runtimeIdentityDisplayFile(peer.file)}
		return empty, false, nil, runtimeIdentityProblem(blocker)
	}
	if current.file != "" {
		return empty, false, nil, fmt.Errorf("target service environment unavailable")
	}
	return projecttypes.Reference{ID: current.row.ID, Name: current.name}, true, nil, nil
}

func runtimeIdentityProblem(blocker *imageDTO.RuntimeIdentityBlocker) error {
	problem := imageDTO.Failure("runtime.target-unavailable", 409)
	problem.Problem.Blocker = imageDTO.SafeRuntimeIdentityBlocker(problem.Problem.Code, blocker)
	return problem
}

// Loader seams that tests count; production uses the real loaders. Tests that swap them must not run in parallel.
var (
	runtimeProjectInputs = (*ProjectService).resolvedProjectInputContext
	runtimeLoadIdentity  = projects.LoadComposeIdentity
)

// Both metadata discovery and executable selection use the same strict identity
// loader. Neither path uses a cache, prepares binds, or writes project files.
func (s *ProjectService) runtimeComposeEvidence(ctx context.Context, row *Project) (string, string, error) {
	ctx = s.composeInputContext(ctx)
	var inputErr error
	ctx, _, inputErr = runtimeProjectInputs(s, ctx, row.Path)
	if inputErr != nil {
		return "", "", inputErr
	}
	selected, err := s.ResolveProjectComposeFile(ctx, row)
	if err != nil {
		return "", "", err
	}
	settings := s.settingsService.GetSettingsOrDefaults(ctx)
	root, err := projects.GetProjectsDirectory(ctx, settings.ProjectsDirectory.Value)
	if err != nil {
		return "", "", err
	}
	autoInject := kit.ParseOrDefault(settings.AutoInjectEnv.Value, false, strconv.ParseBool)
	discovered, err := projects.DetectComposeFile(ctx, root, row.Path)
	if err != nil {
		return "", "", err
	}
	name := projects.NormalizeProjectName(row.Name)
	metadata, missing, err := runtimeLoadIdentity(ctx, discovered, name, root, autoInject)
	if err != nil {
		return "", runtimeMissingIdentityFile(row.Path, missing), err
	}
	model, executableMissing, err := runtimeLoadIdentity(ctx, selected, name, root, autoInject)
	if err != nil {
		return "", runtimeMissingIdentityFile(row.Path, executableMissing), err
	}
	if selected != discovered || model.Name == "" || metadata.Name == "" || model.Name != metadata.Name {
		return "", "", fmt.Errorf("project Compose identity differs between metadata and executable model")
	}
	missing = append(missing, executableMissing...)
	slices.Sort(missing)
	for _, path := range missing {
		relative, err := filepath.Rel(row.Path, path)
		if err == nil && imageDTO.SafeRuntimeFile(filepath.ToSlash(relative)) {
			return model.Name, filepath.ToSlash(relative), nil
		}
	}
	// An empty display path must not turn an unreadable target into an executable one.
	if len(missing) != 0 {
		return model.Name, "\x00", nil
	}
	return model.Name, "", nil
}

func runtimeIdentityDisplayFile(file string) string {
	if imageDTO.SafeRuntimeFile(file) {
		return file
	}
	return ""
}

func runtimeMissingIdentityFile(root string, paths []string) string {
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err == nil && imageDTO.SafeRuntimeFile(filepath.ToSlash(relative)) {
			return filepath.ToSlash(relative)
		}
	}
	return ""
}
