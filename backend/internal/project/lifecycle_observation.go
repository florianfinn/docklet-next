// Source: docklet-next/docs/design/images-updates.md
// Revision: 48915fde5d95ed00c757116410f76617e23e8a50
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Separate deployed configuration bindings from partial-source runtime evidence.

package project

import (
	"context"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	dockernames "go.getarcane.app/docker"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/labels"
	"os"
	"slices"
	"strings"
)

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
	prior := snap.Services
	if prior == nil {
		if baseline, _, e := s.lifecycleStore().Baseline(ctx, snap); e == nil && baseline.SavedRevision == snap.SavedRevision {
			prior = baseline.Services
		}
	}
	snap.Services = map[string]life.ServiceImage{}
	snap.Containers = map[string]life.ServiceImage{}
	snap.AutoExcluded = false
	excluded := dockernames.ExcludedContainerNameSet(s.settingsService.GetStringSetting(ctx, "autoUpdateExcludedContainers", ""))
	for key, svc := range snap.Model.Services {
		if labels.IsUpdateDisabled(svc.Labels) {
			snap.AutoExcluded = true
		}
		img := prior[key]
		if img.Reference != svc.Image {
			img = life.ServiceImage{Reference: svc.Image}
		}
		img.ContainerID = ""
		img.ContainerName = ""
		img.Present = false
		img.Running = false
		img.OneOff = false
		img.Service = key
		snap.Services[key] = img
	}
	// Stable selection of a representative binding; replicas remain independent
	// evidence and never change configuration eligibility merely by their count.
	slices.SortFunc(listed.Items, func(a, b container.Summary) int { return strings.Compare(a.ID, b.ID) })
	for _, item := range listed.Items {
		if item.Labels["com.docker.compose.project"] != snap.Model.Name {
			continue
		}
		inspected, e := c.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if e != nil || inspected.Container.ID != item.ID || inspected.Container.Config == nil {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		key := item.Labels["com.docker.compose.service"]
		oneOff := strings.EqualFold(inspected.Container.Config.Labels["com.docker.compose.oneoff"], "true")
		old, modeled := snap.Services[key]
		id := inspected.Container.Image
		if modeled && !oneOff && (lifecycleSystemLabels(inspected.Container.Config.Labels) || protected[id] || (len(self) >= 12 && strings.HasPrefix(item.ID, self))) {
			return lifecycleFailure("update.invalid-target")
		}
		observed := life.ServiceImage{Reference: inspected.Container.Config.Image, ImageID: id, ContainerID: item.ID, ContainerName: strings.TrimPrefix(inspected.Container.Name, "/"), Service: key, ConfigHash: inspected.Container.Config.Labels["com.docker.compose.config-hash"], OneOff: oneOff, Present: true, Running: inspected.Container.State != nil && inspected.Container.State.Running}
		if guard.ValidateReference(observed.Reference) != nil {
			observed.Reference = id
		}
		snap.Containers[item.ID] = observed
		if !modeled || oneOff {
			continue
		}
		if labels.IsUpdateDisabled(inspected.Container.Config.Labels) || dockernames.ContainerNameExcluded([]string{inspected.Container.Name}, excluded) {
			snap.AutoExcluded = true
		}
		img, e := c.ImageInspect(ctx, id)
		if e != nil || img.InspectResponse.ID != id || img.InspectResponse.Os == "" || img.InspectResponse.Architecture == "" {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		native, e := identity.ResolveContainer(ctx, c, img.InspectResponse, snap.Model.Services[key].Platform, inspected.Container.ImageManifestDescriptor)
		if e != nil {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		observed.Platform = native.Platform.String()
		observed.Digest = native.Digest()
		snap.Containers[item.ID] = observed
		if old.Present && (prior[key].ImageID == "" || old.ImageID == prior[key].ImageID || observed.ImageID != prior[key].ImageID) {
			continue
		}
		observed.Reference = old.Reference
		if observed.Reference == "" {
			observed.Reference = inspected.Container.Config.Image
		}
		if err = guard.ValidateReference(observed.Reference); err != nil {
			return err
		}
		snap.Services[key] = observed
	}
	for key, binding := range snap.Services {
		if binding.Present || binding.Reference == "" {
			continue
		}
		lookup := binding.Reference
		if binding.ImageID != "" {
			lookup = binding.ImageID
		}
		image, e := c.ImageInspect(ctx, lookup)
		if e != nil {
			continue
		}
		requested := snap.Model.Services[key].Platform
		if requested == "" {
			requested = binding.Platform
		}
		native, e := identity.Resolve(ctx, c, image.InspectResponse, requested, nil)
		if e != nil {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		binding.ImageID = native.ID.String()
		binding.Platform = native.Platform.String()
		binding.Digest = native.Digest()
		snap.Services[key] = binding
	}
	return nil
}
