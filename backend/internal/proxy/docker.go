// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy host boundary.

package proxy

import (
	"context"
	"errors"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	"github.com/florianfinn/docklet-next/types/backup"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/acfs"
	"go.getarcane.app/docker/compat"
	"path"
	"strings"
)

// DockerRuntime supplies guarded host primitives for both engines. It never
// parses a proxy configuration and never reads an arbitrary host path.
type DockerRuntime struct {
	Docker   *dockerinternal.DockerClientService
	Settings *settings.SettingsService
	Volumes  *volume.VolumeService
	DB       *database.DB
	Engines  Engines
}

func (d *DockerRuntime) projects() string {
	return d.Settings.GetSettingsConfig().ProjectsDirectory.Value
}
func (d *DockerRuntime) inspect(ctx context.Context, name string) (container.InspectResponse, error) {
	cli, err := d.Docker.GetClient(ctx)
	if err != nil {
		return container.InspectResponse{}, err
	}
	r, err := compat.ContainerInspectWithCompatibility(ctx, cli, name, client.ContainerInspectOptions{})
	return r.Container, err
}
func (d *DockerRuntime) Source(ctx context.Context, c container.InspectResponse, m container.MountPoint) (dto.Source, error) {
	// Writability follows the module's writer (agent project writer or volume/bind
	// helper, Q-SRC), not the proxy container's own mount access: a proxy may mount
	// its configuration read-only and still be configured through Docklet.
	source := dto.Source{Canonical: path.Clean(m.Source), MountTarget: m.Destination, Kind: dto.UnknownSourceKind, MountMode: dto.MountReadOnly}
	if m.RW {
		source.MountMode = dto.MountReadWrite
	}
	cli, err := d.Docker.GetClient(ctx)
	if err != nil {
		return source, err
	}
	st, err := cli.ContainerStatPath(ctx, c.ID, client.ContainerStatPathOptions{Path: m.Destination})
	if err != nil && !errdefs.IsNotFound(err) {
		return source, err // a transient Docker error is not a classification
	}
	if err != nil {
		source.Reason = dto.UnknownSource
		source.ReadOnly = true
		return source, nil
	}
	if !st.Stat.Mode.IsDir() {
		source.Kind = dto.SingleFileSource
		source.ReadOnly = true
		source.Reason = dto.SingleFileMount
		return source, nil
	}
	switch m.Type {
	case mount.TypeVolume:
		source.Kind = dto.NamedVolume
		helperOnly(&source)
	case mount.TypeBind:
		if logical, lerr := acfs.LogicalPath(d.projects(), m.Source); lerr == nil {
			source.Kind = dto.ProjectSource
			// Shown relative to the projects directory; "." is the directory itself.
			source.ProjectPath = strings.TrimPrefix(path.Clean("/"+logical), "/")
			if source.ProjectPath == "" {
				source.ProjectPath = "."
			}
		} else {
			source.Kind = dto.BindSource
			var refused *backup.Problem
			if _, err = bindpath.Check(ctx, cli, d.DB, volumetypes.BindKey(m.Source)); err != nil && !errors.As(err, &refused) {
				return source, err
			}
			if err != nil {
				source.ReadOnly = true
				source.Reason = dto.RefusedBind
			} else {
				helperOnly(&source)
			}
		}
	default:
		source.ReadOnly = true
		source.Reason = dto.UnknownSource
	}
	return source, nil
}

// helperOnly marks a source that only the tools helper could write. The pinned helper
// cannot prove the absence of extended attributes, ACLs or hard links, so such sources
// stay read-only with that reason (Q-HELPER-META); the project writer covers the planned case.
func helperOnly(s *dto.Source) {
	s.ReadOnly = true
	s.Reason = dto.UnsupportedMetadata
}
func (d *DockerRuntime) Check(ctx context.Context, i dto.Instance) error {
	c, err := d.inspect(ctx, i.Identity.ContainerName)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return dto.Fail(dto.ContainerChanged, 409)
		}
		return err
	}
	if c.ID != i.Identity.ContainerID {
		return dto.Fail(dto.ContainerChanged, 409)
	}
	for _, m := range c.Mounts {
		if m.Destination == i.Source.MountTarget {
			s, err := d.Source(ctx, c, m)
			if err != nil {
				return err
			}
			if s.Canonical != i.Source.Canonical || s.Kind != i.Source.Kind || s.ReadOnly != i.Source.ReadOnly {
				return dto.Fail(dto.SourceReadOnly, 409)
			}
			return nil
		}
	}
	return dto.Fail(dto.SourceReadOnly, 409)
}
func (d *DockerRuntime) Epoch(ctx context.Context, i dto.Instance) (bool, string, error) {
	c, err := d.inspect(ctx, i.Identity.ContainerName)
	if errdefs.IsNotFound(err) {
		return false, "", dto.Fail(dto.ContainerChanged, 409) // removed: an observed change
	}
	if err != nil {
		return false, "", err
	}
	if c.ID != i.Identity.ContainerID || c.State == nil {
		return false, "", dto.Fail(dto.ContainerChanged, 409)
	}
	return c.State.Running, c.State.StartedAt, nil
}
func (d *DockerRuntime) Copy(ctx context.Context, i dto.Instance, p string, single bool) ([]dto.File, []dto.OpaqueFile, error) {
	return d.CopyHeaders(ctx, i, p, single, nil)
}
func (d *DockerRuntime) CopyHeaders(ctx context.Context, i dto.Instance, p string, single bool, visit func(hostio.FileHeader) error) ([]dto.File, []dto.OpaqueFile, error) {
	if err := d.Check(ctx, i); err != nil {
		return nil, nil, err
	}
	if !path.IsAbs(p) || path.Clean(p) != p {
		return nil, nil, dto.Fail(dto.InvalidInput, 422)
	}
	if hostio.Secret(p) {
		return []dto.File{}, []dto.OpaqueFile{{Path: p}}, nil
	}
	cli, err := d.Docker.GetClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	r, err := cli.CopyFromContainer(ctx, i.Identity.ContainerID, client.CopyFromContainerOptions{SourcePath: p})
	if err != nil {
		return nil, nil, err
	}
	defer r.Content.Close()
	stop := context.AfterFunc(ctx, func() { _ = r.Content.Close() })
	defer stop()
	return hostio.ReadTarWithHeaders(ctx, r.Content, p, single, visit)
}
func (d *DockerRuntime) Target(ctx context.Context, i dto.Instance, p string) (Bytes, error) {
	if !hostio.Relative(p) || hostio.Secret(p) {
		return Bytes{}, dto.Fail(dto.InvalidInput, 422)
	}
	f, _, err := d.Copy(ctx, i, path.Join(i.Source.MountTarget, p), true)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return Snapshot(nil, false), nil
		}
		return Bytes{}, err
	}
	if len(f) != 1 {
		return Bytes{}, dto.Fail(dto.InvalidInput, 422)
	}
	return Snapshot([]byte(f[0].Content), true), nil
}
func (d *DockerRuntime) Graph(ctx context.Context, i dto.Instance) (map[string]string, error) {
	if err := d.Check(ctx, i); err != nil {
		return nil, err
	}
	e, err := d.Engines.For(i.Kind)
	if err != nil {
		return nil, err
	}
	r, err := e.Read(ctx, i)
	if err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, f := range r.Files {
		hashes[f.Path] = f.Hash
	}
	for _, f := range r.OpaqueFiles {
		hashes[f.Path] = "opaque"
	}
	return hashes, nil
}
func (d *DockerRuntime) Writer(ctx context.Context, i dto.Instance) (hostio.FileWriter, error) {
	if err := d.Check(ctx, i); err != nil {
		return nil, err
	}
	if err := Eligibility(i); err != nil {
		return nil, err
	}
	switch i.Source.Kind {
	case dto.ProjectSource:
		return hostio.ProjectWriter{Root: i.Source.Canonical}, nil
	case dto.BindSource:
		return hostio.HelperWriter{Guard: func(ctx context.Context) error { return d.Check(ctx, i) }, Volumes: d.Volumes, Key: volumetypes.BindKey(i.Source.Canonical)}, nil
	case dto.NamedVolume:
		c, err := d.inspect(ctx, i.Identity.ContainerName)
		if err != nil {
			return nil, err
		}
		for _, m := range c.Mounts {
			if m.Destination == i.Source.MountTarget && m.Type == mount.TypeVolume {
				return hostio.HelperWriter{Guard: func(ctx context.Context) error { return d.Check(ctx, i) }, Volumes: d.Volumes, Key: m.Name}, nil
			}
		}
	}
	return nil, dto.Fail(dto.SourceReadOnly, 409)
}
func (d *DockerRuntime) Stage(ctx context.Context, i Intent) error {
	if !i.New.Present {
		return nil
	}
	w, err := d.Writer(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	return w.Write(ctx, i.TempPath, i.New.Content, i.Mode)
}
func (d *DockerRuntime) Publish(ctx context.Context, i Intent) error {
	w, err := d.Writer(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	// Writer selection can involve helper startup: recheck after that latency.
	b, err := d.Target(ctx, i.Request.Instance, i.Request.Target)
	if err != nil {
		return err
	}
	g, err := d.Graph(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	if b.FileState != i.Old.FileState || !equalGraph(g, i.GraphHashes) {
		return stale(i.Request.Target)
	}
	if helper, ok := w.(hostio.HelperWriter); ok {
		helper.Guard = func(ctx context.Context) error {
			current, err := d.Target(ctx, i.Request.Instance, i.Request.Target)
			if err != nil {
				return err
			}
			graph, err := d.Graph(ctx, i.Request.Instance)
			if err != nil {
				return err
			}
			if current.FileState != i.Old.FileState || !equalGraph(graph, i.GraphHashes) {
				return stale(i.Request.Target)
			}
			return nil
		}
		w = helper
	}
	if i.New.Present {
		return w.Replace(ctx, i.TempPath, i.Request.Target)
	}
	return w.Replace(ctx, i.Request.Target, i.TempPath)
}
func (d *DockerRuntime) Restore(ctx context.Context, i Intent) error {
	w, err := d.Writer(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	b, err := d.Target(ctx, i.Request.Instance, i.Request.Target)
	if err != nil {
		return err
	}
	if b.FileState != i.Old.FileState && b.FileState != i.New.FileState {
		return dto.Fail(dto.TargetChanged, 409)
	}
	if b.FileState == i.Old.FileState {
		return nil
	}
	if helper, ok := w.(hostio.HelperWriter); ok {
		helper.Guard = func(ctx context.Context) error {
			current, err := d.Target(ctx, i.Request.Instance, i.Request.Target)
			if err != nil {
				return err
			}
			if current.FileState != i.New.FileState {
				return dto.Fail(dto.TargetChanged, 409)
			}
			return nil
		}
		w = helper
	}
	if !i.Old.Present {
		return w.Remove(ctx, i.Request.Target)
	}
	if err = w.Remove(ctx, i.TempPath); err != nil {
		return err
	}
	if err = w.Write(ctx, i.TempPath, i.Old.Content, i.Mode); err != nil {
		return err
	}
	b, err = d.Target(ctx, i.Request.Instance, i.Request.Target)
	if err != nil {
		return err
	}
	if b.FileState != i.New.FileState {
		return dto.Fail(dto.TargetChanged, 409)
	}
	return w.Replace(ctx, i.TempPath, i.Request.Target)
}
func (d *DockerRuntime) Cleanup(ctx context.Context, i Intent) error {
	// Read-only instances never staged anything; avoid selecting a writer for
	// pre-admission/eligibility refusals. Persisted write intents are checked.
	if !i.Request.Instance.Eligibility.Writable {
		return nil
	}
	w, err := d.cleanupWriter(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range []string{i.TempPath, i.MarkerPath} {
		if p != "" {
			errs = append(errs, w.Remove(ctx, p))
		}
	}
	if i.StagingPath != "" {
		errs = append(errs, w.RemoveTree(ctx, i.StagingPath))
	}
	return errors.Join(errs...)
}
func (d *DockerRuntime) Discover(ctx context.Context) ([]dto.Discovery, error) {
	rows, err := d.Docker.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	return d.ProjectDiscovery(ctx, rows)
}

// imageKind names the engine from an image reference's repository base. A tag
// colon follows the last slash; a colon before it is a registry port.
func imageKind(image string) dto.Kind {
	repo, _, _ := strings.Cut(strings.ToLower(image), "@")
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	switch path.Base(repo) {
	case "traefik":
		return dto.Traefik
	case "caddy":
		return dto.Caddy
	}
	return ""
}

// ProjectDiscovery never mutates the shared coalesced listing or its nested mounts.
func (d *DockerRuntime) ProjectDiscovery(ctx context.Context, rows []container.Summary) ([]dto.Discovery, error) {
	out := []dto.Discovery{}
	for _, r := range rows {
		k := imageKind(r.Image)
		if k == "" {
			continue
		}
		c, err := d.inspect(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		if c.State == nil {
			continue
		}
		row := dto.Discovery{Kind: k, Image: r.Image, Identity: dto.Identity{ContainerID: c.ID, ContainerName: strings.TrimPrefix(c.Name, "/"), StartedAt: c.State.StartedAt}, Sources: []dto.Source{}}
		for _, m := range c.Mounts {
			s, err := d.Source(ctx, c, m)
			if err != nil {
				return nil, err
			}
			row.Sources = append(row.Sources, s)
		}
		out = append(out, row)
	}
	return out, nil
}

// LastVerified distinguishes ownership from a verified snapshot. A created but
// unverified file is owned, with no restore candidate until its first verified edit.
func (d *DockerRuntime) LastVerified(ctx context.Context, i dto.Instance, p string) (Bytes, bool, error) {
	if err := d.Check(ctx, i); err != nil {
		return Bytes{}, false, err
	}
	var row verifiedFile
	err := d.DB.WithContext(ctx).First(&row, "source = ? AND path = ?", "host\x00"+i.Source.Canonical, p).Error
	if missing(err) {
		return Bytes{}, false, nil
	}
	if err != nil {
		return Bytes{}, false, err
	}
	if row.Hash == "" {
		return Bytes{}, true, nil
	}
	return Snapshot(row.Content, true), true, nil
}

// Rebinding is restricted to removal of the intent's own transient names. A
// recreated container must expose the identical canonical source and mount kind;
// this never changes the registered identity or permits target publication.
func (d *DockerRuntime) cleanupWriter(ctx context.Context, i dto.Instance) (hostio.FileWriter, error) {
	w, original := d.Writer(ctx, i)
	if original == nil {
		return w, nil
	}
	p, ok := errors.AsType[*dto.Problem](original)
	if !ok || p.Code != dto.ContainerChanged {
		return nil, original
	}
	current, err := d.Rebind(ctx, i)
	if err != nil {
		return nil, original
	}
	return d.Writer(ctx, current)
}

// Rebind follows a recreated container only when the same mount target still
// holds the same configuration source; any other change keeps the old identity refused.
func (d *DockerRuntime) Rebind(ctx context.Context, i dto.Instance) (dto.Instance, error) {
	if err := d.Check(ctx, i); err == nil {
		return i, nil
	}
	c, err := d.inspect(ctx, i.Identity.ContainerName)
	if errdefs.IsNotFound(err) {
		c, err = d.inspect(ctx, i.Identity.ContainerID)
	}
	if err != nil && !errdefs.IsNotFound(err) {
		return i, err // a daemon failure is not a container change
	}
	if err != nil || c.State == nil {
		return i, dto.Fail(dto.ContainerChanged, 409)
	}
	// Every refusal means the container no longer serves the recorded source.
	for _, mount := range c.Mounts {
		if mount.Destination != i.Source.MountTarget {
			continue
		}
		source, err := d.Source(ctx, c, mount)
		if err != nil {
			return i, err
		}
		compared := i.Source
		compared.MountMode = source.MountMode // recovery retains its established proxy-access-independent contract
		if !sameSource(source, compared) {
			return i, dto.Fail(dto.ContainerChanged, 409)
		}
		current := i
		current.Identity = dto.Identity{ContainerID: c.ID, ContainerName: strings.TrimPrefix(c.Name, "/"), StartedAt: c.State.StartedAt}
		return current, nil
	}
	return i, dto.Fail(dto.ContainerChanged, 409)
}
