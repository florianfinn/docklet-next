// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Fresh, read-only project identity for runtime control.

package project

import (
	"cmp"
	"context"
	"errors"
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

// Every persisted row contributes fresh evidence before duplicate precedence or
// an acknowledgement is evaluated. Archived rows retain ownership too.
func (s *ProjectService) ResolveRuntimeProjectIdentityAcknowledged(ctx context.Context, projectID, composeName, acknowledgement string) (projecttypes.Reference, bool, *imageDTO.RuntimePeerOverrides, error) {
	empty := projecttypes.Reference{}
	if projectID == "" && composeName == "" {
		return empty, false, nil, nil
	}
	var rows []Project
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return empty, false, nil, fmt.Errorf("list runtime project identities: %w", err)
	}
	type evidence struct {
		row        *Project
		name, file string
		err        error
	}
	all := make([]evidence, 0, len(rows))
	owners := map[string][]int{}
	match := -1
	var targetErr error
	var unknown []imageDTO.RuntimeIdentityPeer
	for i := range rows {
		row := &rows[i]
		name, file, err := s.runtimeComposeEvidence(ctx, row)
		all = append(all, evidence{row, name, file, err})
		if err == nil && name != "" {
			owners[name] = append(owners[name], i)
			if projectID != "" && row.ID == projectID || projectID == "" && name == composeName {
				match = i
			}
		}
		if err != nil {
			if row.ID == projectID {
				targetErr = err
			}
			reason := "compose.identity-unreadable"
			var missing *projects.MissingServiceEnvironmentError
			if errors.As(err, &missing) {
				reason = "compose.env-file-missing"
			}
			if UnsupportedUpdateLayout(err) != nil {
				reason = "compose.selection-unsupported"
			}
			unknown = append(unknown, imageDTO.RuntimeIdentityPeer{PeerID: row.ID, Name: imageDTO.RuntimeDisplayName(row.Name, row.ID), Reason: reason, Archived: row.IsArchived, File: runtimeIdentityDisplayFile(file)})
		}
	}
	if targetErr != nil {
		return empty, false, nil, fmt.Errorf("read runtime project identity: %w", targetErr)
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
	if len(owners[current.name]) != 1 {
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
	uncertainty := imageDTO.NewRuntimeUncertainty(unknown)
	if uncertainty != nil && uncertainty.Digest != acknowledgement {
		first := uncertainty.Peers[0]
		blocker := &imageDTO.RuntimeIdentityBlocker{Name: first.Name, PeerID: first.PeerID, Archived: first.Archived, Reason: "compose.name-undeterminable", Confirmable: true, Uncertainty: uncertainty}
		return empty, false, nil, runtimeIdentityProblem(blocker)
	}
	var used *imageDTO.RuntimePeerOverrides
	if uncertainty != nil {
		slices.SortFunc(unknown, func(a, b imageDTO.RuntimeIdentityPeer) int { return cmp.Compare(a.PeerID, b.PeerID) })
		used = &imageDTO.RuntimePeerOverrides{Count: len(unknown), Peers: slices.Clone(unknown[:min(len(unknown), 64)])}
	}
	return projecttypes.Reference{ID: current.row.ID, Name: current.name}, true, used, nil
}

func runtimeIdentityProblem(blocker *imageDTO.RuntimeIdentityBlocker) error {
	problem := imageDTO.Failure("runtime.target-unavailable", 409)
	problem.Problem.Blocker = imageDTO.SafeRuntimeIdentityBlocker(problem.Problem.Code, blocker)
	return problem
}

// Both metadata discovery and executable selection use the same strict identity
// loader. Neither path uses a cache, prepares binds, or writes project files.
func (s *ProjectService) runtimeComposeEvidence(ctx context.Context, row *Project) (string, string, error) {
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
	metadata, missing, err := projects.LoadComposeIdentity(ctx, discovered, name, root, autoInject)
	if err != nil {
		return "", runtimeMissingIdentityFile(row.Path, missing), err
	}
	model, executableMissing, err := projects.LoadComposeIdentity(ctx, selected, name, root, autoInject)
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
