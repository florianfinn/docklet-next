// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original per-service deploy stage observation and behavioural coverage.

package projects

import (
	"slices"
	"strconv"
	"strings"

	"github.com/docker/compose/v5/pkg/api"
)

// deployServiceRowLimit bounds the published rows; the remainder is marked truncated.
const deployServiceRowLimit = 64

// Observed stage states. An absent stage was never observed; consumers derive
// pending or skipped from the run state and later stages.
const (
	deployStageRunning = "running"
	deployStageDone    = "done"
	deployStageError   = "error"
)

// deployStages records the observed stages of one container.
type deployStages struct {
	create, start, health string
	// completed: a one-shot container exited successfully while Compose waited for it.
	completed bool
}

// healthcheckDeclared reports whether Compose declares a healthcheck: nil when
// the image may define one, false when the service disables it.
func healthcheckDeclared(check *bool) any {
	if check == nil {
		return nil
	}
	return *check
}

// BindImage ties an image pull to a service when the pulled reference differs
// from the Compose image, as for digest pins. Bindings survive rebinding.
func (r *DeployReporter) BindImage(service, ref string) {
	if r == nil || service == "" || ref == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.bindings[service]; !exists && len(r.bindings) >= 512 {
		return
	}
	r.bindings[service] = ref
	r.dirty = true
}

// Build records a deploy-time image build of one service.
func (r *DeployReporter) Build(service, state string) {
	if r == nil || (state != deployStageRunning && state != deployStageDone && state != deployStageError) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.builds[service] == state {
		return
	}
	r.builds[service] = state
	r.dirty = true
	r.flushLocked(false)
}

// stageService resolves container events and Compose's structured create
// errors ("service:<name>:<replica>") to a configured service, or "".
func (r *DeployReporter) stageService(id string) string {
	if len(id) > 256 {
		return ""
	}
	if rest, ok := strings.CutPrefix(id, "service:"); ok {
		name, replica, found := strings.Cut(rest, ":")
		number, err := strconv.Atoi(replica)
		if found && err == nil && number > 0 && slices.ContainsFunc(r.services, func(s deployService) bool { return s.name == name }) {
			return name
		}
		return ""
	}
	if len(id) < 11 || id[:10] != "Container " {
		return ""
	}
	return r.resourceService(id)
}

// observeStage maps one Compose container event onto that container's stages.
// Unchanged containers report Running, which means they are already started.
// A start that fails without an event stays running; consumers show it as
// interrupted once the run settles.
func (r *DeployReporter) observeStage(event api.Resource) {
	if r.stageService(event.ID) == "" {
		return
	}
	stages := r.containers[event.ID]
	if stages == nil {
		if len(r.containers) >= 512 {
			return
		}
		stages = &deployStages{}
		r.containers[event.ID] = stages
	}
	switch {
	case event.Status == api.Error:
		switch {
		case stages.health == deployStageRunning || stages.start == deployStageDone:
			stages.health = deployStageError
		case stages.start == deployStageRunning || stages.create == deployStageDone:
			stages.start = deployStageError
		default:
			stages.create = deployStageError
		}
	case event.Text == api.StatusCreating || event.Text == "Recreate" || event.Text == "Recreating":
		stages.create = deployStageRunning
	case event.Text == api.StatusCreated || event.Text == "Recreated":
		stages.create = deployStageDone
	case event.Text == api.StatusStarting:
		stages.start = deployStageRunning
	case event.Text == api.StatusStarted || event.Text == api.StatusRunning:
		stages.start = deployStageDone
	case event.Text == api.StatusWaiting:
		stages.health = deployStageRunning
	case event.Text == api.StatusHealthy:
		stages.health = deployStageDone
	case event.Text == api.StatusExited && event.Status == api.Done:
		// service_completed_successfully: a zero exit ends the wait; failures arrive as errors.
		stages.health, stages.completed = deployStageDone, true
	default:
		return
	}
	r.dirty = true
}

// mergeStage combines replica states: any error wins, then any running work.
func mergeStage(current, next string) string {
	switch {
	case current == deployStageError || next == deployStageError:
		return deployStageError
	case current == deployStageRunning || next == deployStageRunning:
		return deployStageRunning
	case current == deployStageDone || next == deployStageDone:
		return deployStageDone
	}
	return ""
}

func (r *DeployReporter) servicesLocked() map[string]any {
	merged := make(map[string]*deployStages, len(r.services))
	for id, stages := range r.containers {
		name := r.stageService(id)
		row := merged[name]
		if row == nil {
			row = &deployStages{}
			merged[name] = row
		}
		row.create, row.start, row.health = mergeStage(row.create, stages.create), mergeStage(row.start, stages.start), mergeStage(row.health, stages.health)
		row.completed = row.completed || stages.completed
	}
	all := slices.Clone(r.services)
	for name := range r.bindings {
		if !slices.ContainsFunc(all, func(s deployService) bool { return s.name == name }) {
			all = append(all, deployService{name: name})
		}
	}
	services := slices.SortedFunc(slices.Values(all), func(a, b deployService) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})
	rows := make([]map[string]any, 0, min(len(services), deployServiceRowLimit))
	for _, service := range services[:min(len(services), deployServiceRowLimit)] {
		row := map[string]any{"name": service.name}
		if check := healthcheckDeclared(service.check); check != nil {
			row["check"] = check
		}
		image := service.image
		if bound, ok := r.bindings[service.name]; ok {
			image = bound
		}
		if layers, ok := r.images[image]; ok && image != "" && len(layers) > 0 {
			done, current, total, known := deployLayerSum(layers)
			pull := map[string]any{"layersDone": done, "layersTotal": len(layers)}
			if known && !r.incomplete {
				pull["bytesDone"], pull["bytesTotal"] = current, total
			}
			row["pull"] = pull
		}
		if build := r.builds[service.name]; build != "" {
			row["build"] = build
		}
		if stages := merged[service.name]; stages != nil {
			if stages.completed {
				row["completed"] = true
			}
			for key, value := range map[string]string{"create": stages.create, "start": stages.start, "health": stages.health} {
				if value != "" {
					row[key] = value
				}
			}
		}
		rows = append(rows, row)
	}
	result := map[string]any{"v": 1, "services": rows}
	// Image preparation succeeded; unobserved pulls and builds were not needed.
	if r.prepared {
		result["prepared"] = true
	}
	if len(services) > deployServiceRowLimit {
		result["truncated"] = true
	}
	return result
}

// deployLayerSum totals one image's layers; known is false while any layer lacks a byte total.
func deployLayerSum(layers map[string]*deployLayer) (done int, current, total int64, known bool) {
	known = true
	for _, layer := range layers {
		if layer.done {
			done++
		}
		known = known && layer.known
		current = saturatingDeployAdd(current, layer.current)
		total = saturatingDeployAdd(total, layer.total)
	}
	return done, current, total, known
}
