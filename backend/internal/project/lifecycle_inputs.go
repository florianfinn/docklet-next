// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Complete bounded Compose input capture and admitted revision-checked writes.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"go.getarcane.app/sys/cgroup"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	edits "github.com/florianfinn/docklet-next/backend/internal/project/children/update"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"go.getarcane.app/acfs"
	updatertypes "go.getarcane.app/updater/types"
	"go.yaml.in/yaml/v4"
)

func (s *ProjectService) lifecycleStore() life.Store { return life.Store{DB: s.db} }
func lifecycleFailure(code string) error {
	status := 409
	switch code {
	case "update.invalid-target":
		status = 400
	case "update.target-not-found", "restore.not-found":
		status = 404
	case "update.persistence-failed", "image.tag-failed", "update.deploy-failed", "restore.deploy-failed":
		status = 500
	case "image.pull-failed":
		status = 502
	case "update.check-unavailable":
		status = 503
	}
	return imageDTO.Failure(code, status)
}
func targetLifecycleIdentity(p *Project) string { return life.Hash([]any{p.ID, p.CreatedAt, p.Path}) }

func (s *ProjectService) lifecycleInputs(ctx context.Context, p *Project) (*life.Snapshot, error) {
	if err := s.EnsureProjectPathUnderRoot(ctx, p, false); err != nil {
		return nil, lifecycleFailure("update.snapshot-unavailable")
	}
	path, err := s.ResolveProjectComposeFile(ctx, p)
	if err != nil {
		return nil, lifecycleFailure("update.snapshot-unavailable")
	}
	cfg := s.settingsService.GetSettingsOrDefaults(ctx)
	root, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return nil, lifecycleFailure("update.snapshot-unavailable")
	}
	inject, _ := strconv.ParseBool(cfg.AutoInjectEnv.Value)
	var deps projectDTO.ComposeDependencies
	load := func() (*compose.Project, error) {
		return projects.LoadComposeProject(ctx, path, projects.NormalizeProjectName(p.Name), root, inject, s.projectPathMapper(ctx), nil, nil, false, &deps, nil, nil)
	}
	model, err := load()
	if err != nil {
		return nil, lifecycleFailure("update.snapshot-unavailable")
	}
	// Unsupported include/multi-file compositions cannot claim complete capture.
	if len(model.ComposeFiles) != 1 {
		return nil, lifecycleFailure("update.image-binding-unsupported")
	}
	paths := slices.Concat(model.ComposeFiles, deps.EnvFiles, []string{filepath.Join(p.Path, ".env")})
	for _, v := range model.Configs {
		if v.File != "" {
			paths = append(paths, v.File)
		}
	}
	for _, v := range model.Secrets {
		if v.File != "" {
			paths = append(paths, v.File)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	read := func() ([]life.File, error) {
		var files []life.File
		total := 0
		for _, path := range paths {
			rel, e := filepath.Rel(p.Path, path)
			if e != nil || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			content, e := acfs.ReadFile(ctx, p.Path, "/"+filepath.ToSlash(rel))
			present := true
			if errors.Is(e, os.ErrNotExist) && rel == ".env" {
				present = false
				content = nil
				e = nil
			}
			total += len(content)
			if e != nil || total > 4<<20 {
				return nil, lifecycleFailure("update.snapshot-unavailable")
			}
			files = append(files, life.File{Path: filepath.ToSlash(rel), Content: content, Present: present})
		}
		return files, nil
	}
	files, err := read()
	if err != nil {
		return nil, err
	}
	next, err := load()
	if err != nil || life.Hash(next) != life.Hash(model) {
		return nil, lifecycleFailure("update.target-changed")
	}
	again, err := read()
	if err != nil || life.Hash(files) != life.Hash(again) {
		return nil, lifecycleFailure("update.target-changed")
	}
	// An include may resolve into a merged model while ComposeFiles lists only
	// its primary source; reject that source explicitly.
	primary, _ := filepath.Rel(p.Path, path)
	for _, f := range files {
		if f.Path == filepath.ToSlash(primary) {
			var document yaml.Node
			if yaml.Unmarshal(f.Content, &document) != nil {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			if lifecycleYAMLHas(&document, "include") || lifecycleYAMLHas(&document, "extends") {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
		}
	}
	return &life.Snapshot{TargetID: p.ID, TargetIdentity: targetLifecycleIdentity(p), Files: files, Model: model, SavedRevision: life.Hash([]any{files, model})}, nil
}
func lifecycleYAMLHas(n *yaml.Node, key string) bool {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				return true
			}
		}
	}
	for _, c := range n.Content {
		if lifecycleYAMLHas(c, key) {
			return true
		}
	}
	return false
}
func (s *ProjectService) lifecycleObserve(ctx context.Context, snap *life.Snapshot) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("update.snapshot-unavailable")
	}
	protected := map[string]bool{}
	if refs := s.imageService.ImageGuard().SystemReferences; refs != nil {
		for _, ref := range refs() {
			if ref == "" {
				continue
			}
			img, e := c.ImageInspect(ctx, ref)
			if errdefs.IsNotFound(e) {
				continue
			}
			if e != nil {
				return lifecycleFailure("update.snapshot-unavailable")
			}
			protected[img.InspectResponse.ID] = true
		}
	}
	self, _ := libarcane.CurrentContainerInspectTarget(cgroup.CurrentContainerID, os.Hostname)
	listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return lifecycleFailure("update.snapshot-unavailable")
	}
	snap.Services = make(map[string]life.ServiceImage, len(snap.Model.Services))
	for key, svc := range snap.Model.Services {
		snap.Services[key] = life.ServiceImage{Reference: svc.Image}
	}
	for _, item := range listed.Items {
		if item.Labels["com.docker.compose.project"] != snap.Model.Name {
			continue
		}
		key := item.Labels["com.docker.compose.service"]
		old, ok := snap.Services[key]
		if !ok || old.Present {
			return lifecycleFailure("update.baseline-unavailable")
		}
		inspected, e := c.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if e != nil || inspected.Container.ID != item.ID || inspected.Container.Config == nil {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		id := inspected.Container.Image
		if lifecycleSystemLabels(inspected.Container.Config.Labels) || protected[id] || (len(self) >= 12 && strings.HasPrefix(item.ID, self)) {
			return lifecycleFailure("update.invalid-target")
		}

		img, e := c.ImageInspect(ctx, id)
		if e != nil || img.InspectResponse.ID != id {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		digest := ""
		if len(img.InspectResponse.RepoDigests) > 0 {
			digest = img.InspectResponse.RepoDigests[0]
		}
		if old.Reference == "" {
			old.Reference = inspected.Container.Config.Image
		}
		if err = guard.ValidateReference(old.Reference); err != nil {
			return err
		}
		if img.InspectResponse.Os == "" || img.InspectResponse.Architecture == "" {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		old.ContainerID = inspected.Container.ID
		old.ContainerName = strings.TrimPrefix(inspected.Container.Name, "/")
		old.ImageID = id
		old.Digest = digest
		old.Platform = img.InspectResponse.Os + "/" + img.InspectResponse.Architecture
		if img.InspectResponse.Variant != "" {
			old.Platform += "/" + img.InspectResponse.Variant
		}
		old.Present = true
		old.Running = inspected.Container.State != nil && inspected.Container.State.Running
		snap.Services[key] = old
	}
	return nil
}
func (s *ProjectService) lifecycleWrite(ctx context.Context, p *Project, current, desired *life.Snapshot, actor user.Actor) error {
	s.contentMu.Lock()
	defer s.contentMu.Unlock()
	fresh, err := s.lifecycleInputs(ctx, p)
	if err != nil || fresh.SavedRevision != current.SavedRevision {
		return lifecycleFailure("update.target-changed")
	}
	if actor.ID == "" || actor.ID == user.SystemUser.ID {
		return lifecycleFailure("update.invalid-target")
	}
	root, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return lifecycleFailure("update.target-changed")
	}
	for _, f := range desired.Files {
		if !f.Present {
			continue
		}
		if _, err := acfs.LogicalPath(root, p.Path); err != nil {
			return lifecycleFailure("update.target-changed")
		}
		if err := acfs.Write(ctx, p.Path, "/"+f.Path, f.Content, acfs.WriteOptions{Mode: 0600, InPlace: true}); err != nil {
			return lifecycleFailure("update.target-changed")
		}
	}
	for _, f := range desired.Files {
		if !f.Present {
			if err := acfs.Remove(ctx, p.Path, "/"+f.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return lifecycleFailure("update.target-changed")
			}
		}
	}
	s.invalidateProjectCaches(p.ID)
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, p.ID, p.Name, actor, database.JSON{"action": "image_lifecycle_files", "expectedRevision": current.SavedRevision}, "could not record image lifecycle file write")
	return nil
}
func lifecycleVersionFiles(snapshot *life.Snapshot, selections []dto.ImageSelection) (*life.Snapshot, error) {
	body, _ := json.Marshal(snapshot)
	var desired life.Snapshot
	if json.Unmarshal(body, &desired) != nil {
		return nil, lifecycleFailure("update.image-binding-unsupported")
	}
	changes := map[string]updatertypes.ServiceImageChange{}
	validations := map[string]updatertypes.ServiceImageChange{}
	for _, sel := range selections {
		svc, ok := snapshot.Model.Services[sel.Service]
		if !ok || svc.Build != nil || strings.ContainsAny(sel.SourceReference, "$@") {
			return nil, lifecycleFailure("update.image-binding-unsupported")
		}
		// Rebuilds are validated against the literal field as well, but never saved.
		validations[sel.Service] = updatertypes.ServiceImageChange{ExpectedRef: sel.SourceReference, TargetRef: sel.SourceReference}
		if sel.Kind == "version" {
			changes[sel.Service] = updatertypes.ServiceImageChange{ExpectedRef: sel.SourceReference, TargetRef: sel.Reference}
		}
	}
	primary := filepath.Base(snapshot.Model.ComposeFiles[0])
	found := false
	for i, f := range desired.Files {
		if f.Path == primary {
			// The resolved model cannot prove an interpolated source was literal.
			var document yaml.Node
			if yaml.Unmarshal(f.Content, &document) != nil {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			var check func(*yaml.Node) bool
			check = func(n *yaml.Node) bool {
				if n.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(n.Content); j += 2 {
						if n.Content[j].Value == "image" && strings.Contains(n.Content[j+1].Value, "$") {
							return false
						}
					}
				}
				for _, c := range n.Content {
					if !check(c) {
						return false
					}
				}
				return true
			}
			if !check(&document) {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			_, _, err := edits.PrepareProjectServiceImages(f.Content, snapshot.Model, validations)
			if err != nil {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			updated := f.Content
			if len(changes) > 0 {
				updated, _, err = edits.PrepareProjectServiceImages(f.Content, snapshot.Model, changes)
			}
			if err != nil {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			desired.Files[i].Content = updated
			found = true
		}
	}
	if !found {
		return nil, lifecycleFailure("update.image-binding-unsupported")
	}
	for _, sel := range selections {
		if sel.Kind == "version" {
			svc := desired.Model.Services[sel.Service]
			svc.Image = sel.Reference
			desired.Model.Services[sel.Service] = svc
		}
	}
	return &desired, nil
}
func lifecycleFilesEqual(a, b *life.Snapshot) bool {
	return bytes.Equal(mustLifecycleJSON(a.Files), mustLifecycleJSON(b.Files))
}
func mustLifecycleJSON(v any) []byte { b, _ := json.Marshal(v); return b }
