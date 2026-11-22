// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage.

package projects

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
)

type deployReporterKey struct{}

// DeployReporter observes one executor-owned run. Callbacks are serialized and
// synchronous; the owner joins its periodic flush before the final Flush.
type DeployReporter struct {
	mu           sync.Mutex
	phase        string
	images       map[string]map[string]*deployLayer
	layers       int
	incomplete   bool
	resources    map[string]deployResource
	services     []deployService
	activity     map[string]any
	activitySeen bool
	dirty        bool
	lastPatch    time.Time
	now          func() time.Time
	appendPhase  func(string, bool)
	patch        func(map[string]any)
}

type deployResource struct{ label, service string }
type deployService struct{ name, prefix, custom string }

type deployLayer struct {
	done           bool
	current, total int64
	known          bool
}

func NewDeployReporter(phase func(string, bool), patch func(map[string]any), now func() time.Time) *DeployReporter {
	return &DeployReporter{images: make(map[string]map[string]*deployLayer), resources: make(map[string]deployResource), appendPhase: phase, patch: patch, now: now}
}

func WithDeployReporter(ctx context.Context, reporter *DeployReporter) context.Context {
	return context.WithValue(ctx, deployReporterKey{}, reporter)
}

func DeployReporterFromContext(ctx context.Context) *DeployReporter {
	reporter, _ := ctx.Value(deployReporterKey{}).(*DeployReporter)
	return reporter
}

// ConfigureProject binds container observations to services so replicas are
// counted once. Unknown resource names carry no service-count claim.
func (r *DeployReporter) ConfigureProject(project *composetypes.Project) {
	if r == nil || project == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, service := range project.Services {
		if len(r.services) >= 512 {
			break
		}
		r.services = append(r.services, deployService{name: name, prefix: project.Name + "-" + name + "-", custom: service.ContainerName})
	}
}

func (r *DeployReporter) resourceService(id string) string {
	name := strings.TrimPrefix(id, "Container ")
	if len(r.services) == 0 {
		return name
	}
	for _, service := range r.services {
		if service.custom != "" && name == service.custom {
			return service.name
		}
		if suffix, ok := strings.CutPrefix(name, service.prefix); ok {
			replica, err := strconv.Atoi(suffix)
			if err == nil && replica > 0 {
				return service.name
			}
		}
	}
	return ""
}

func (r *DeployReporter) Phase(phase string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase == phase || (phase != "pull" && phase != "deploy") {
		return
	}
	r.phase = phase
	r.appendPhase(phase, false)
}

// Preparation can fail before any pull is needed. Name that failed operation
// without inventing a pull for a cached image or a never policy.
func (r *DeployReporter) PrepareFailed() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase == "" {
		r.phase = "deploy"
		r.appendPhase("deploy", false)
	}
}

func (r *DeployReporter) PullWarning() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase == "pull" {
		r.appendPhase("pull", true)
	}
}

var deployLayerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// ObservePull accepts daemon JSON before display formatting. Extraction must not
// reset downloaded bytes; invalid or absent totals suppress the byte projection.
func (r *DeployReporter) ObservePull(image string, frame []byte) {
	if r == nil {
		return
	}
	var message struct {
		ID     string          `json:"id"`
		Status string          `json:"status"`
		Detail json.RawMessage `json:"progressDetail"`
	}
	if json.Unmarshal(frame, &message) != nil || !deployLayerID.MatchString(message.ID) {
		return
	}
	switch message.Status {
	case "Pulling fs layer", "Waiting", "Downloading", "Verifying Checksum", "Download complete", "Extracting", "Pull complete", "Already exists":
	default:
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	layers, exists := r.images[image]
	if !exists {
		if len(r.images) >= 64 {
			r.markIncomplete()
			return
		}
		layers = make(map[string]*deployLayer)
		r.images[image] = layers
	}
	layer := layers[message.ID]
	if layer == nil {
		if r.layers >= 512 {
			r.markIncomplete()
			return
		}
		layer = &deployLayer{}
		layers[message.ID] = layer
		r.layers++
	}
	if message.Status == "Downloading" {
		var detail struct {
			Current *int64 `json:"current"`
			Total   *int64 `json:"total"`
		}
		layer.known = json.Unmarshal(message.Detail, &detail) == nil && detail.Current != nil && detail.Total != nil && *detail.Current >= 0 && *detail.Total >= 0 && *detail.Current <= *detail.Total
		if layer.known {
			layer.current, layer.total = *detail.Current, *detail.Total
		}
	}
	if message.Status == "Download complete" && layer.known {
		layer.current = layer.total
	}
	if message.Status == "Already exists" && !layer.known {
		// A cached layer transfers nothing; its zero bytes are known, so it cannot hide the other layers' sum.
		layer.known, layer.current, layer.total = true, 0, 0
	}
	if message.Status == "Pull complete" || message.Status == "Already exists" {
		layer.done = true
		if layer.known {
			layer.current = layer.total
		}
	}
	r.dirty = true
	r.flushLocked(false)
}

// Crossing the retention bounds makes a whole observed byte sum unknowable.
func (r *DeployReporter) markIncomplete() {
	if !r.incomplete {
		r.incomplete = true
		r.dirty = true
		r.flushLocked(false)
	}
}

func saturatingDeployAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func (r *DeployReporter) pullLocked() map[string]any {
	done := 0
	var current, total int64
	known := r.layers > 0 && !r.incomplete
	for _, layers := range r.images {
		for _, layer := range layers {
			if layer.done {
				done++
			}
			known = known && layer.known
			current = saturatingDeployAdd(current, layer.current)
			total = saturatingDeployAdd(total, layer.total)
		}
	}
	result := map[string]any{"v": 1, "images": len(r.images), "layersDone": done, "layersTotal": r.layers}
	if known {
		result["bytesDone"], result["bytesTotal"] = current, total
	}
	return result
}

// Resource events are observations inside deploy, never successful trace steps.
func (r *DeployReporter) On(events ...api.Resource) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase != "deploy" {
		return
	}
	for _, event := range events {
		if event.Status != api.Working {
			active, exists := r.resources[event.ID]
			delete(r.resources, event.ID)
			if exists && r.activity != nil && r.activity["label"] == active.label {
				count := r.activeServices(active.label)
				r.activity = nil
				if count > 0 {
					r.activity = map[string]any{"v": 1, "label": active.label, "services": count}
				}
				r.dirty = true
			}
			continue
		}
		label := ""
		switch event.Text {
		case api.StatusCreating, "Recreate", "Recreating":
			label = "create"
		case api.StatusStopping:
			label = "stop"
		case api.StatusStarting:
			label = "start"
		case api.StatusWaiting:
			label = "wait"
		}
		// Compose also reports networks and volumes; only container work is service activity.
		if label == "" || len(event.ID) > 256 || len(event.ID) < 11 || event.ID[:10] != "Container " {
			continue
		}
		if _, exists := r.resources[event.ID]; !exists && len(r.resources) >= 512 {
			continue
		}
		service := r.resourceService(event.ID)
		if service == "" {
			continue
		}
		r.resources[event.ID] = deployResource{label: label, service: service}
		count := r.activeServices(label)
		r.activity = map[string]any{"v": 1, "label": label, "services": count}
		r.activitySeen = true
		r.dirty = true
	}
	r.flushLocked(false)
}

func (r *DeployReporter) activeServices(label string) int {
	observed := make(map[string]bool)
	for _, active := range r.resources {
		if active.label == label {
			observed[active.service] = true
		}
	}
	return len(observed)
}

// Flush publishes pending observations at most once per second; final bypasses
// the throttle once, after all producers have joined and before settlement.
func (r *DeployReporter) Flush(final bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushLocked(final)
}

func (r *DeployReporter) flushLocked(final bool) {
	if !r.dirty || (!final && !r.lastPatch.IsZero() && r.now().Sub(r.lastPatch) < time.Second) {
		return
	}
	patch := map[string]any{}
	if r.layers > 0 {
		patch["deployPull"] = r.pullLocked()
	}
	if r.activitySeen {
		patch["deployActivity"] = r.activity
	}
	r.patch(patch)
	r.lastPatch = r.now()
	r.dirty = false
}

type deployEventTee struct {
	plain    api.EventProcessor
	reporter *DeployReporter
}

func (t deployEventTee) Start(ctx context.Context, operation string) { t.plain.Start(ctx, operation) }
func (t deployEventTee) On(events ...api.Resource)                   { t.reporter.On(events...); t.plain.On(events...) }
func (t deployEventTee) Done(operation string, success bool)         { t.plain.Done(operation, success) }
