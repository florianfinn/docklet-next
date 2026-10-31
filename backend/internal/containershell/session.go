// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original hub shell acceptance, activity and handle identity registry.

// Package containershell owns hub shell sessions independently of agent identity.
package containershell

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"
	"sync"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/user"
)

func ParseQuery(values url.Values) (container.ShellQuery, error) {
	q := container.ShellQuery{Shell: "auto"}
	for key, values := range values {
		if key != "shell" && key != "cols" && key != "rows" || len(values) != 1 {
			return q, invalid()
		}
	}
	if values.Has("shell") {
		q.Shell = values.Get("shell")
	}
	if q.Shell != "auto" && q.Shell != "sh" && q.Shell != "bash" {
		return q, invalid()
	}
	for key, target := range map[string]**int{"cols": &q.Cols, "rows": &q.Rows} {
		if !values.Has(key) {
			continue
		}
		value, err := strconv.Atoi(values.Get(key))
		if err != nil || value < 1 || value > 1000 {
			return q, invalid()
		}
		*target = &value
	}
	return q, nil
}
func invalid() error { return &container.Problem{Code: "container.shell-invalid", Status: 400} }

type Registry struct {
	mu       sync.Mutex
	handles  map[*Handle]struct{}
	activity activitylib.Service
	owner    context.Context
}

func New(service activitylib.Service, owners ...context.Context) *Registry {
	r := &Registry{handles: map[*Handle]struct{}{}, activity: service}
	if len(owners) > 0 {
		r.owner = owners[0]
	}
	return r
}

type Handle struct {
	registry   *Registry
	actor      string
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	mu         sync.Mutex
	closer     func()
	closed     bool
	reason     string
	activityID string
	lease      *sessionlife.Lease
	stop       func() bool
	stopOwner  func() bool
}

// Accept validates before creating any activity or upgrading. Its returned context
// retains the hub browser identity; callers must release even on upgrade failure.
func (r *Registry) Accept(ctx context.Context, values url.Values, actor *user.Actor, env, containerID string) (context.Context, *Handle, error) {
	if _, err := ParseQuery(values); err != nil {
		return ctx, nil, err
	}
	h := &Handle{registry: r, reason: "container.shell-unavailable"}
	if actor != nil {
		h.actor = actor.ID
	}
	h.ctx, h.cancel = context.WithCancel(ctx)
	if r.activity != nil {
		resource := "container"
		entry, err := r.activity.StartActivity(ctx, activitylib.StartRequest{EnvironmentID: env, Type: activity.TypeResourceAction, ResourceType: &resource, ResourceID: &containerID, StartedBy: actor, LatestMessage: "Shell started", Metadata: map[string]any{"operation": "container.shell"}})
		if err != nil {
			h.cancel()
			return ctx, nil, err
		}
		h.activityID = entry.ID
		if tracker, ok := r.activity.(activitylib.Tracker); ok {
			h.ctx = tracker.Track(h.ctx, h.activityID)
		}
	}
	r.mu.Lock()
	r.handles[h] = struct{}{}
	r.mu.Unlock()
	h.stop = context.AfterFunc(ctx, h.Close)
	if r.owner != nil {
		h.stopOwner = context.AfterFunc(r.owner, h.Close)
	}
	lease, err := sessionlife.Register(ctx, h.Close)
	h.lease = lease
	if err != nil {
		h.Release()
		return ctx, nil, err
	}
	return ws.WithTerminal(h.ctx, h), h, nil
}
func (h *Handle) SetCloser(closer func()) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		closer()
		return
	}
	h.closer = closer
	h.mu.Unlock()
}
func (h *Handle) Observe(reason string) {
	// Only bounded protocol reasons reach activity persistence.
	switch reason {
	case "", "container.shell-invalid", "container.shell-protocol", "container.shell-unavailable", "container.shell-orphaned", "container.shell-not-running":
	default:
		reason = "container.shell-unavailable"
	}
	h.mu.Lock()
	h.reason = reason
	h.mu.Unlock()
}
func (h *Handle) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	closer := h.closer
	h.mu.Unlock()
	h.cancel()
	if closer != nil {
		closer()
	}
}

// Release removes only this handle. A delayed callback cannot find or remove a
// replacement session, even for the same actor, environment and container.
func (h *Handle) Release() {
	h.once.Do(func() {
		h.Close()
		if h.stopOwner != nil {
			h.stopOwner()
		}
		if h.stop != nil {
			h.stop()
		}
		if h.lease != nil {
			h.lease.Release()
		}
		h.registry.mu.Lock()
		delete(h.registry.handles, h)
		h.registry.mu.Unlock()
		h.mu.Lock()
		reason := h.reason
		h.mu.Unlock()
		if h.activityID != "" {
			status := activity.StatusSuccess
			if reason != "" {
				status = activity.StatusFailed
			}
			if _, err := h.registry.activity.CompleteActivity(context.WithoutCancel(h.ctx), h.activityID, status, reason, nil); err != nil {
				slog.ErrorContext(h.ctx, "Failed to complete shell activity", "error", err)
			}
		}
	})
}
func (r *Registry) CloseByUser(userID string) {
	r.mu.Lock()
	var handles []*Handle
	for h := range r.handles {
		if h.actor == userID {
			handles = append(handles, h)
		}
	}
	r.mu.Unlock()
	for _, h := range handles {
		h.Close()
	}
}

// ActiveCount reports registered handles, including sessions still joining cleanup.
func (r *Registry) ActiveCount() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.handles) }
