// Source: docklet-next/issues/317
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded dependency-aware definition services for tracked stacks without containers. M34 #547: load problems for every stack and Compose name peers. M34 #548: recorded Compose environment inputs.

package project

import (
	"context"
	"slices"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"golang.org/x/sync/errgroup"
)

func (s *ProjectService) fillDefinitionServices(ctx context.Context, records []Project, rows []projecttypes.Details, env *projectMetadataEnv) {
	var group errgroup.Group
	group.SetLimit(4)
	for i := range records {
		if records[i].Path == "" || rows[i].Status == projecttypes.StatusUnknown {
			continue
		}
		group.Go(func() error {
			model, err := s.getCachedComposeProject(ctx, &records[i], env)
			if err != nil {
				// Every stack shows its own load problem, running stacks included (#547).
				problem := projects.ClassifyLoadError(err)
				rows[i].DefinitionHint = "definition-invalid"
				rows[i].DefinitionReason = "compose.identity-unreadable"
				_, file, identityErr := s.runtimeComposeEvidence(ctx, &records[i])
				switch {
				case file != "" && identityErr == nil:
					rows[i].DefinitionReason = "compose.env-file-missing"
					rows[i].DefinitionFile = runtimeIdentityDisplayFile(file)
				case UnsupportedUpdateLayout(identityErr) != nil:
					rows[i].DefinitionReason = "compose.selection-unsupported"
				case problem.Reason == "compose.env-label-invalid" || problem.Reason == "compose.env-label-conflict":
					rows[i].DefinitionReason = problem.Reason
				case problem.Reason == "compose.variable-missing":
					rows[i].DefinitionReason = problem.Reason
					rows[i].DefinitionVariable = problem.Variable
				}
				return nil
			}
			if len(rows[i].RuntimeServices) != 0 {
				return nil
			}
			services := make([]projecttypes.RuntimeService, 0, len(model.Services))
			for _, name := range model.ServiceNames() {
				service := model.Services[name]
				services = append(services, projecttypes.RuntimeService{Name: name, Image: service.Image, Status: projecttypes.ServiceNotCreated})
			}
			rows[i].RuntimeServices = services
			rows[i].ServiceCount = len(services)
			return nil
		})
	}
	_ = group.Wait()
	s.fillComposeNamePeers(ctx, rows, env)
}

// fillComposeNamePeers warns about stacks sharing an effective Compose name.
// All persisted rows count, archived and off-page ones included. Readable
// models give their name; unreadable ones only literal evidence. The warning
// never authorizes or refuses anything.
func (s *ProjectService) fillComposeNamePeers(ctx context.Context, rows []projecttypes.Details, env *projectMetadataEnv) {
	if len(rows) == 0 || projects.PeerNamesHidden(ctx) {
		return
	}
	root, _ := s.GetProjectsDirectory(ctx)
	var all []Project
	if err := s.db.WithContext(ctx).Find(&all).Error; err != nil {
		return
	}
	names := make([]string, len(all))
	var group errgroup.Group
	group.SetLimit(4)
	for i := range all {
		if all[i].Path == "" {
			continue
		}
		group.Go(func() error {
			if model, err := s.getCachedComposeProject(ctx, &all[i], env); err == nil {
				names[i] = projects.NormalizeProjectName(model.Name)
			} else if input, _ := projects.ResolveComposeInputs(s.composeInputContext(ctx), root, all[i].Path, false); input == nil || input.State == "none" {
				// Recorded inputs may select another name or file set.
				names[i] = projects.PeerLiteralName(all[i].Path)
			}
			return nil
		})
	}
	_ = group.Wait()
	byName := map[string][]int{}
	byID := map[string]int{}
	for i, name := range names {
		byID[all[i].ID] = i
		if name != "" {
			byName[name] = append(byName[name], i)
		}
	}
	for r := range rows {
		i, ok := byID[rows[r].ID]
		if !ok || names[i] == "" {
			continue
		}
		var peers []string
		for _, j := range byName[names[i]] {
			if j != i && len(peers) < 16 {
				peers = append(peers, imageDTO.RuntimeDisplayName(all[j].Name, all[j].ID))
			}
		}
		slices.Sort(peers)
		rows[r].ComposeNamePeers = peers
	}
}
