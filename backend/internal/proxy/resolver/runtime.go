// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver

import (
	"context"
	"errors"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"gorm.io/gorm"
)

func itoa(n int) string { return strconv.Itoa(n) }

type Host interface {
	Prepare(context.Context, dto.ResolverApplyRequest) (Target, error)
	Preflight(context.Context, Target) error
	Recreate(context.Context, Target) error
	Settle(context.Context, Target) error
	Replacement(context.Context, Target, dto.Instance) (dto.Instance, error)
	Observe(context.Context, dto.Instance, string, bool, time.Time) error
}

// Runtime connects the independent oracle to existing read ports. The splice
// port is supplied by Traefik so the engine's original span editor is shared.
type Runtime struct {
	DB                *gorm.DB
	ProjectsDirectory string
	AutoInject        bool
	Splice            Splice
	Inspect           func(context.Context, string) (container.InspectResponse, error)
	List              func(context.Context) ([]container.Summary, error)
	Inventory         func(context.Context, dto.Instance) (dto.Inventory, error)
	Logs              func(context.Context, dto.Instance, time.Time, func(string) error) error
	Source            func(context.Context, container.InspectResponse, container.MountPoint) (dto.Source, error)
	Up                func(context.Context, *compose.Project, []string, bool, bool, bool, time.Duration) error
	Now               func() time.Time
	Wait              func(context.Context, time.Duration) error
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (r *Runtime) pause(ctx context.Context, d time.Duration) error {
	if r.Wait != nil {
		return r.Wait(ctx, d)
	}
	return wait(ctx, d)
}
func (r *Runtime) project(ctx context.Context, name string) (Target, error) {
	var rows []struct{ ID, Path string }
	if err := r.DB.WithContext(ctx).Table("projects").Select("id,path").Where("COALESCE(compose_project_name, name) = ? AND is_archived = ?", name, false).Limit(2).Scan(&rows).Error; err != nil {
		return Target{}, dto.Fail(dto.TargetUnclear, 409)
	}
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Path, r.ProjectsDirectory+"/") {
		return Target{}, layout(dto.UnmanagedProject)
	}
	return Target{ProjectID: rows[0].ID, Directory: rows[0].Path, Project: name, AutoInject: r.AutoInject}, nil
}

// recordedInputs refuses stacks whose containers record an env file: the
// resolver's .env edit would be inert and its recreate would drop the label.
func (r *Runtime) recordedInputs(ctx context.Context, dir string) error {
	if r.List == nil {
		return nil
	}
	ctx = projects.WithComposeContainerLabels(ctx, func(ctx context.Context) ([]map[string]string, error) {
		rows, err := r.List(ctx)
		labels := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			labels = append(labels, row.Labels)
		}
		return labels, err
	})
	input, _ := projects.ResolveComposeInputs(ctx, r.ProjectsDirectory, dir, false)
	if input != nil && input.State != "none" {
		return dto.Fail(dto.EnvConflict, 422)
	}
	return nil
}
func (r *Runtime) Prepare(ctx context.Context, q dto.ResolverApplyRequest) (Target, error) {
	i := q.Instance
	if i.Kind != dto.Traefik || i.Source.Kind != dto.ProjectSource || i.Source.ReadOnly {
		return Target{}, layout(dto.UnmanagedProject)
	}
	c, err := r.Inspect(ctx, i.Identity.ContainerID)
	if err != nil {
		return Target{}, dto.Fail(dto.AgentUnavailable, 503)
	}
	if c.Config == nil || c.State == nil || !c.State.Running || c.ID != i.Identity.ContainerID {
		return Target{}, dto.Fail(dto.ContainerChanged, 409)
	}
	project, service := c.Config.Labels["com.docker.compose.project"], c.Config.Labels["com.docker.compose.service"]
	t, err := r.project(ctx, project)
	if err != nil {
		return t, err
	}
	t.Service = service
	if err = r.recordedInputs(ctx, t.Directory); err != nil {
		return t, err
	}
	if i.Source.Canonical != t.Directory && !strings.HasPrefix(i.Source.Canonical, t.Directory+"/") {
		return t, layout(dto.StaticOutsideProject)
	}
	t.Source = i.Source
	if service == "" {
		return t, layout(dto.UnmanagedProject)
	}
	if _, err = r.Replacement(ctx, t, i); err != nil {
		return t, layout(dto.MultipleReplicas)
	}
	file, err := projects.DetectComposeFile(ctx, r.ProjectsDirectory, t.Directory)
	if err != nil {
		return t, dto.Fail(dto.ComposeLoadFailed, 422)
	}
	if path.Dir(file) != t.Directory {
		return t, layout(dto.MultipleComposeFiles)
	}
	t.Compose = path.Base(file)
	cf, err := Read(t.Directory, t.Compose)
	if err != nil {
		return t, err
	}
	cf.Kind = "compose"
	ef, err := Read(t.Directory, ".env")
	if err != nil {
		return t, layout(dto.EnvNotRegular)
	}
	ef.Kind = "env"
	if _, _, err = Environment(ef.Before, nil); err != nil {
		return t, err
	}
	if err = ValidateRaw(cf.Before, t.Service); err != nil {
		return t, err
	}
	if projects.DetectComposeOverrideFile(t.Directory) != "" {
		return t, layout(dto.MultipleComposeFiles)
	}
	model, err := projects.LoadComposeProject(ctx, file, t.Project, r.ProjectsDirectory, r.AutoInject, nil, nil, nil, false, nil, nil, nil)
	if err != nil {
		return t, dto.Fail(dto.ComposeLoadFailed, 422)
	}
	if len(model.ComposeFiles) != 1 || model.ComposeFiles[0] != file {
		return t, layout(dto.MultipleComposeFiles)
	}
	if len(projects.NamespaceDependents(model, []string{service})) > 0 {
		return t, layout(dto.NamespaceDependent)
	}
	svc, ok := model.Services[service]
	// Without a compose command the container runs the image's default command
	// (implicit static file layout); an explicit command must match exactly.
	if !ok || svc.Command != nil && !slices.Equal([]string(svc.Command), []string(c.Config.Cmd)) {
		return t, dto.Fail(dto.TargetUnclear, 409)
	}
	// The scoped recreate drops other services, so the selected service must not
	// borrow another service's namespaces or volumes.
	for _, mode := range []string{svc.NetworkMode, svc.Ipc, svc.Pid} {
		if strings.HasPrefix(mode, "service:") {
			return t, layout(dto.NamespaceDependent)
		}
	}
	if len(svc.VolumesFrom) > 0 {
		return t, layout(dto.NamespaceDependent)
	}
	inv, err := r.Inventory(ctx, i)
	if err != nil {
		return t, dto.Fail(dto.IncompleteInventory, 422)
	}
	if err := staticLayout(c, inv); err != nil {
		return t, err
	}
	if !inv.Eligibility.Writable {
		return t, dto.Fail(dto.IncompleteInventory, 422)
	}
	// Mounted storage authority follows the most specific destination, not the
	// selected configuration mount, and must be writable from both sides.
	var storage *container.MountPoint
	for j := range c.Mounts {
		m := &c.Mounts[j]
		if q.Request.Storage == m.Destination || strings.HasPrefix(q.Request.Storage, strings.TrimSuffix(m.Destination, "/")+"/") {
			if storage == nil || len(m.Destination) > len(storage.Destination) {
				storage = m
			}
		}
	}
	if storage == nil || !storage.RW {
		return t, dto.Fail(dto.StorageNotWritable, 422)
	}
	fd, _, e := parent(storage.Source, strings.TrimPrefix(q.Request.Storage, strings.TrimSuffix(storage.Destination, "/")+"/"))
	if e != nil {
		return t, dto.Fail(dto.StorageNotWritable, 422)
	}
	if err = accessDirectory(fd); err != nil {
		return t, dto.Fail(dto.StorageNotWritable, 422)
	}
	if inv.RootFile != "" {
		var selected *container.MountPoint
		for j := range c.Mounts {
			m := &c.Mounts[j]
			if inv.RootFile == m.Destination || strings.HasPrefix(inv.RootFile, strings.TrimSuffix(m.Destination, "/")+"/") {
				if selected == nil || len(m.Destination) > len(selected.Destination) {
					selected = m
				}
			}
		}
		if selected == nil {
			return t, layout(dto.StaticOutsideProject)
		}
		local := path.Join(selected.Source, strings.TrimPrefix(inv.RootFile, selected.Destination))
		if !strings.HasPrefix(local, t.Directory+"/") {
			return t, layout(dto.StaticOutsideProject)
		}
		t.Static = strings.TrimPrefix(local, t.Directory+"/")
		sf, e := Read(t.Directory, t.Static)
		if e != nil {
			return t, e
		}
		sf.Kind = "static"
		t.Files = append(t.Files, sf)
	}

	t.Files = append(t.Files, cf, ef)
	return Build(ctx, t, q.Request, inv, r.Splice)
}
func (r *Runtime) Preflight(ctx context.Context, t Target) error {
	current, err := r.project(ctx, t.Project)
	if err != nil || current.ProjectID != t.ProjectID || current.Directory != t.Directory {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	if err = r.recordedInputs(ctx, t.Directory); err != nil {
		return err
	}
	file, err := projects.DetectComposeFile(ctx, r.ProjectsDirectory, t.Directory)
	if err != nil || file != path.Join(t.Directory, t.Compose) {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	cf, err := Read(t.Directory, t.Compose)
	if err != nil {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	if err = ValidateRaw(cf.Before, t.Service); err != nil {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	ef, err := Read(t.Directory, ".env")
	if err != nil {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	if _, _, err = Environment(ef.Before, nil); err != nil {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	model, err := projects.LoadComposeProject(ctx, file, t.Project, r.ProjectsDirectory, t.AutoInject, nil, nil, nil, false, nil, nil, nil)
	if err != nil {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	if len(model.ComposeFiles) != 1 || model.ComposeFiles[0] != file {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	if _, ok := model.Services[t.Service]; !ok {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	if len(projects.NamespaceDependents(model, []string{t.Service})) != 0 {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	rows, err := r.serviceContainers(ctx, t)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return dto.Fail(dto.IdentityUnclear, 409)
	}
	c, err := r.Inspect(ctx, rows[0].ID)
	if err != nil {
		return dto.Fail(dto.AgentUnavailable, 503)
	}
	if c.Config == nil || c.Config.Labels["com.docker.compose.project"] != t.Project || c.Config.Labels["com.docker.compose.service"] != t.Service {
		return dto.Fail(dto.IdentityUnclear, 409)
	}
	matched := false
	for _, m := range c.Mounts {
		if m.Destination == t.Source.MountTarget {
			s, err := r.Source(ctx, c, m)
			if err != nil {
				return dto.Fail(dto.AgentUnavailable, 503)
			}
			matched = s.Canonical == t.Source.Canonical && s.Kind == t.Source.Kind
		}
	}
	if !matched {
		return dto.Fail(dto.IdentityUnclear, 409)
	}
	return nil
}
func (r *Runtime) Recreate(ctx context.Context, t Target) error {
	model, err := projects.LoadComposeProject(ctx, path.Join(t.Directory, t.Compose), t.Project, r.ProjectsDirectory, t.AutoInject, nil, nil, nil, false, nil, nil, nil)
	if err != nil {
		return dto.Fail(dto.ComposeLoadFailed, 422)
	}
	selected, err := Scoped(model, t.Service)
	if err != nil {
		return dto.Fail(dto.ComposeLoadFailed, 422)
	}
	// Compose's standard HTTP detachment respects an app lifecycle context, so
	// this deadline bounds the whole call as well as its Wait stage.
	run, cancel := context.WithTimeout(utils.WithAppLifecycleContext(context.Background()), 60*time.Second)
	defer cancel()
	run = projects.WithComposeOrphansPreserved(run)
	if r.Up != nil {
		err = r.Up(run, selected, []string{t.Service}, false, true, false, 60*time.Second)
	} else {
		err = projects.ComposeUp(run, selected, []string{t.Service}, false, true, false, nil, 60*time.Second)
	}
	if err != nil {
		return dto.Fail(dto.ComposeUpFailed, 422)
	}
	return nil
}
func (r *Runtime) serviceContainers(ctx context.Context, t Target) ([]container.Summary, error) {
	all, err := r.List(ctx)
	if err != nil {
		return nil, dto.Fail(dto.AgentUnavailable, 503)
	}
	out := []container.Summary{}
	for _, c := range all {
		if c.Labels["com.docker.compose.project"] == t.Project && c.Labels["com.docker.compose.service"] == t.Service {
			out = append(out, c)
		}
	}
	return out, nil
}
func (r *Runtime) Settle(ctx context.Context, t Target) error {
	limit, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		rows, err := r.serviceContainers(limit, t)
		if err != nil {
			return err
		}
		pending := false
		for _, c := range rows {
			if slices.Contains([]string{"created", "restarting", "removing"}, string(c.State)) {
				pending = true
			}
			info, err := r.Inspect(limit, c.ID)
			if err != nil {
				return dto.Fail(dto.AgentUnavailable, 503)
			}
			if info.State != nil && info.State.Health != nil && info.State.Health.Status == "starting" {
				pending = true
			}
		}
		if !pending {
			return nil
		}
		if r.pause(limit, 2*time.Second) != nil {
			return dto.Fail(dto.EffectUnsettled, 409)
		}
	}
}
func (r *Runtime) Replacement(ctx context.Context, t Target, old dto.Instance) (dto.Instance, error) {
	rows, err := r.serviceContainers(ctx, t)
	if err != nil {
		return old, err
	}
	running := []container.Summary{}
	for _, c := range rows {
		if c.State == "running" {
			running = append(running, c)
		}
	}
	if len(running) != 1 {
		return old, dto.Fail(dto.IdentityUnclear, 409)
	}
	c, err := r.Inspect(ctx, running[0].ID)
	if err != nil {
		return old, dto.Fail(dto.AgentUnavailable, 503)
	}
	if c.Config == nil || c.State == nil || !c.State.Running {
		return old, dto.Fail(dto.IdentityUnclear, 409)
	}
	for _, m := range c.Mounts {
		if m.Destination != old.Source.MountTarget {
			continue
		}
		s, err := r.Source(ctx, c, m)
		if err != nil {
			return old, dto.Fail(dto.AgentUnavailable, 503)
		}
		if s.Canonical != old.Source.Canonical || s.Kind != old.Source.Kind {
			return old, dto.Fail(dto.IdentityUnclear, 409)
		}
		old.Identity = dto.Identity{ContainerID: c.ID, ContainerName: strings.TrimPrefix(c.Name, "/"), StartedAt: c.State.StartedAt}
		return old, nil
	}
	return old, dto.Fail(dto.IdentityUnclear, 409)
}
func (r *Runtime) Observe(ctx context.Context, i dto.Instance, name string, present bool, since time.Time) error {
	started, err := time.Parse(time.RFC3339Nano, i.Identity.StartedAt)
	if err != nil || !started.After(since) {
		return dto.Fail(dto.ProxyRejected, 422)
	}
	deadline := r.now().Add(15 * time.Second)
	window, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		if err = r.startup(window, i, started); err != nil {
			if code(err, "") == dto.LogsUnavailable && window.Err() == context.DeadlineExceeded && ctx.Err() == nil {
				break
			}
			return err
		}
		if !r.now().Before(deadline) {
			break
		}
		if err = r.pause(window, time.Second); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil && (window.Err() == context.DeadlineExceeded || !r.now().Before(deadline)) {
				break
			}
			return dto.Fail(dto.LogsUnavailable, 422)
		}
	}
	// Timer expiry completes the startup window. A separate bounded read closes
	// the epoch and checks its full log interval without reusing an expired timer.
	if remaining := deadline.Sub(r.now()); remaining > 0 {
		if r.pause(ctx, remaining) != nil {
			return dto.Fail(dto.LogsUnavailable, 422)
		}
	}
	final, finish := context.WithTimeout(ctx, 5*time.Second)
	defer finish()
	if err = r.startup(final, i, started); err != nil {
		return err
	}
	inv, err := r.Inventory(final, i)
	if err != nil || !inv.Eligibility.Writable || inv.Identity.ContainerID != i.Identity.ContainerID || inv.Identity.StartedAt != i.Identity.StartedAt || slices.Contains(inv.Resolvers, name) != present {
		return dto.Fail(dto.ProxyRejected, 422)
	}
	return nil
}
func (r *Runtime) startup(ctx context.Context, i dto.Instance, since time.Time) error {
	c, err := r.Inspect(ctx, i.Identity.ContainerID)
	if err != nil {
		return dto.Fail(dto.LogsUnavailable, 422)
	}
	if c.ID != i.Identity.ContainerID || c.State == nil || !c.State.Running || c.State.StartedAt != i.Identity.StartedAt {
		return dto.Fail(dto.ProxyRejected, 422)
	}
	rejected := false
	if r.Logs(ctx, i, since, func(line string) error {
		if strings.Contains(strings.ToLower(line), "command traefik error") {
			rejected = true
		}
		return nil
	}) != nil {
		return dto.Fail(dto.LogsUnavailable, 422)
	}
	if rejected {
		return dto.Fail(dto.ProxyRejected, 422)
	}
	return nil
}

func (r *Runtime) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runtime) LockSource(ctx context.Context, i dto.Instance) (string, error) {
	c, err := r.Inspect(ctx, i.Identity.ContainerID)
	if err != nil || c.Config == nil {
		return "", dto.Fail(dto.AgentUnavailable, 503)
	}
	t, err := r.project(ctx, c.Config.Labels["com.docker.compose.project"])
	if err != nil {
		return "", err
	}
	if i.Source.Canonical != t.Directory && !strings.HasPrefix(i.Source.Canonical, t.Directory+"/") {
		return "", layout(dto.StaticOutsideProject)
	}
	return t.Directory, nil
}

// Confirmation uses the project directory derived and persisted at admission.
// It needs no Docker effect or running container, including after a failed restore.
func (r *Runtime) Confirmation(ctx context.Context, i dto.Instance, files []dto.ResolverFile) (Target, error) {
	var rows []struct{ ID, Path string }
	if i.Source.ProjectPath == "" {
		return Target{}, dto.Fail(dto.TargetUnclear, 409)
	}
	if err := r.DB.WithContext(ctx).Table("projects").Select("id,path").Where("path = ?", i.Source.ProjectPath).Limit(2).Scan(&rows).Error; err != nil {
		return Target{}, dto.Fail(dto.AgentUnavailable, 503)
	}
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Path, r.ProjectsDirectory+"/") || (i.Source.Canonical != rows[0].Path && !strings.HasPrefix(i.Source.Canonical, rows[0].Path+"/")) {
		return Target{}, dto.Fail(dto.TargetUnclear, 409)
	}
	t := Target{ProjectID: rows[0].ID, Directory: rows[0].Path, Source: i.Source}
	for _, file := range files {
		if !relative(file.Path) {
			return t, dto.Fail(dto.TargetUnclear, 409)
		}
		t.Files = append(t.Files, File{ResolverFile: file})
	}
	if len(t.Files) == 0 {
		return t, dto.Fail(dto.TargetUnclear, 409)
	}
	return t, nil
}

func staticLayout(c container.InspectResponse, inv dto.Inventory) error {
	if strings.EqualFold(path.Ext(inv.RootFile), ".toml") {
		return layout(dto.TOMLStatic)
	}
	for _, arg := range c.Config.Entrypoint {
		if strings.HasPrefix(arg, "--") {
			return layout(dto.EntrypointFlags)
		}
	}
	for _, value := range c.Config.Env {
		if strings.HasPrefix(value, "TRAEFIK_") {
			if inv.RootFile != "" {
				return layout(dto.MixedStaticSources)
			}
			return layout(dto.EnvironmentStatic)
		}
	}
	if inv.RootFile != "" {
		for _, arg := range c.Config.Cmd {
			if strings.HasPrefix(arg, "--") && !strings.HasPrefix(strings.ToLower(arg), "--configfile") {
				return layout(dto.MixedStaticSources)
			}
		}
	}
	return nil
}
