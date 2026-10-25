// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Validate updater dependencies and preserve stop signals in the ephemeral deployment model.

package project

import (
	"context"
	compose "github.com/compose-spec/compose-go/v2/types"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/inventory"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/moby/moby/client"
	"go.getarcane.app/updater/labels"
	"maps"
	"slices"
	"strings"
)

func lifecycleOrderedModel(snap *life.Snapshot) (*compose.Project, error) {
	model, err := snap.Model.WithServicesEnabled()
	if err != nil {
		return nil, lifecycleFailure("update.invalid-target")
	}
	aliases := map[string]string{}
	for key, svc := range model.Services {
		if lifecycleSystemLabels(svc.Labels) {
			return nil, lifecycleFailure("update.invalid-target")
		}
		for _, name := range []string{key, svc.ContainerName, snap.Services[key].ContainerName} {
			if name == "" {
				continue
			}
			if prior, ok := aliases[name]; ok && prior != key {
				return nil, lifecycleFailure("update.invalid-target")
			}
			aliases[name] = key
		}
	}
	for key, svc := range model.Services {
		if signal := labels.StopSignal(svc.Labels); signal != "" {
			svc.StopSignal = signal
		}
		if svc.DependsOn == nil {
			svc.DependsOn = compose.DependsOnConfig{}
		}
		for label, value := range svc.Labels {
			if !strings.EqualFold(label, labels.LabelDependsOn) {
				continue
			}
			for _, name := range strings.Split(value, ",") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				dep, ok := aliases[name]
				if !ok {
					return nil, lifecycleFailure("update.invalid-target")
				}
				svc.DependsOn[dep] = compose.ServiceDependency{Condition: compose.ServiceConditionStarted, Required: true}
			}
		}
		model.Services[key] = svc
	}
	visited := map[string]int{}
	var visit func(string) error
	visit = func(key string) error {
		if visited[key] == 1 {
			return lifecycleFailure("update.invalid-target")
		}
		if visited[key] == 2 {
			return nil
		}
		visited[key] = 1
		for dep := range model.Services[key].DependsOn {
			if _, ok := model.Services[dep]; !ok {
				return lifecycleFailure("update.invalid-target")
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[key] = 2
		return nil
	}
	for key := range model.Services {
		if err := visit(key); err != nil {
			return nil, err
		}
	}
	return model, nil
}

func (s *ProjectService) lifecycleStopSignals(ctx context.Context, source *life.Snapshot) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("update.target-changed")
	}
	model, err := lifecycleOrderedModel(source)
	if err != nil {
		return err
	}
	ordered := []string{}
	visited := map[string]bool{}
	var visit func(string)
	visit = func(key string) {
		if visited[key] {
			return
		}
		visited[key] = true
		for dep := range model.Services[key].DependsOn {
			visit(dep)
		}
		ordered = append(ordered, key)
	}
	for key := range model.Services {
		visit(key)
	}
	slices.Reverse(ordered)
	for _, key := range ordered {
		signal := labels.StopSignal(model.Services[key].Labels)
		if signal == "" {
			continue
		}
		candidates := source.Containers
		if len(candidates) == 0 {
			candidates = map[string]life.ServiceImage{key: source.Services[key]}
		}
		for _, id := range slices.Sorted(maps.Keys(candidates)) {
			img := candidates[id]
			if len(source.Containers) > 0 && (img.Service != key || img.OneOff) {
				continue
			}
			if !img.Present || !img.Running {
				continue
			}
			if _, err = c.ContainerStop(ctx, img.ContainerID, client.ContainerStopOptions{Signal: signal}); err != nil {
				return lifecycleFailure("update.deploy-failed")
			}
			img.Running = false
			if len(source.Containers) > 0 {
				source.Containers[id] = img
			}
		}
		img := source.Services[key]
		img.Running = false
		source.Services[key] = img
	}
	return nil
}

func lifecycleSystemLabels(values map[string]string) bool {
	return labels.IsArcaneContainer(values) || values[inventory.SystemLabel] != "" || strings.EqualFold(values[libarcane.InternalResourceLabel], "true")
}

func lifecycleSourceStopSignal(source *life.Snapshot, key string) string {
	service := source.Model.Services[key]
	if signal := labels.StopSignal(service.Labels); signal != "" {
		return signal
	}
	return service.StopSignal
}
