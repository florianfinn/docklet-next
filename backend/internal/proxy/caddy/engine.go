// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Caddy engine using the shared guarded runtime and journal.

package caddy

import (
	"context"
	"encoding/json/v2"
	"errors"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
)

// Runtime is the subset of DockerRuntime used by the engine. Production callers
// pass Module.Runtime; tests replace only Docker transport or filesystem storage.
type Runtime interface {
	InspectContainer(context.Context, dto.Instance) (container.InspectResponse, error)
	Source(context.Context, container.InspectResponse, container.MountPoint) (dto.Source, error)
	Copy(context.Context, dto.Instance, string, bool) ([]dto.File, []dto.OpaqueFile, error)
	Exec(context.Context, dto.Instance, string, string, string, string) (hostio.ExecResult, error)
	Writer(context.Context, dto.Instance) (hostio.FileWriter, error)
	LastVerified(context.Context, dto.Instance, string) (proxy.Bytes, bool, error)
	RestartContainer(context.Context, proxy.Intent) (string, error)
	Epoch(context.Context, dto.Instance) (bool, string, error)
	Logs(context.Context, dto.Instance, time.Time, func(string) error) error
}
type engine struct {
	activated sync.Map
	runtime   Runtime
	now       func() time.Time
	wait      func(context.Context, time.Duration) error
}

// New returns an unregistered engine. Register it before Module.Start.
func New(runtime Runtime) proxy.Engine {
	return &engine{runtime: runtime, now: time.Now, wait: func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}}
}

var _ proxy.Engine = (*engine)(nil)
var _ Runtime = (*proxy.DockerRuntime)(nil)

type state struct {
	Root        string          `json:"root"`
	Admin       string          `json:"admin"`
	AdminOff    bool            `json:"adminOff"`
	Before      hostio.Response `json:"before"`
	ProbeReason dto.Reason      `json:"probeReason,omitempty"`
	OldSite     *Site           `json:"oldSite,omitempty"`
	NewSite     *Site           `json:"newSite,omitempty"`
	// Rename marks an edit that changes only the route name: the running graph is
	// unchanged, so the admin readback is the only evidence it can produce.
	Rename bool `json:"rename,omitempty"`
}

func decode(i proxy.Intent) (state, error) {
	var s state
	err := json.Unmarshal(i.Engine, &s)
	return s, err
}
func (e *engine) Inventory(ctx context.Context, i dto.Instance) (dto.Inventory, error) {
	g, err := e.load(ctx, i)
	if err != nil {
		return g.inventory, err
	}
	if g.inventory.Eligibility.Writable && g.inventory.Resume && !g.inventory.AdminOff && g.inventory.PersistConfig {
		diverged, err := e.diverged(ctx, i, g.inventory)
		if err != nil {
			eligibility := refused(reason(err, dto.AdminUnavailable), g.root)
			var diagnostic *dto.Problem
			if errors.As(err, &diagnostic) {
				eligibility.Path = diagnostic.Path
				eligibility.Line = diagnostic.Line
				eligibility.Detail = diagnostic.Detail
			}
			g.inventory.Eligibility = eligibility
			return g.inventory, nil
		}
		g.inventory.RunningDiverged = diverged
	}
	return g.inventory, nil
}
func (e *engine) Read(ctx context.Context, i dto.Instance) (dto.ReadResult, error) {
	g, err := e.load(ctx, i)
	out := dto.ReadResult{Generation: i.Generation, Inventory: g.inventory, Eligibility: g.inventory.Eligibility, Files: []dto.File{}, OpaqueFiles: g.opaque, Routes: []dto.Route{}}
	if err != nil {
		return out, err
	}
	paths := []string{}
	for p := range g.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := g.files[p]
		if hostio.Relative(f.Path) {
			b, owned, err := e.runtime.LastVerified(ctx, i, f.Path)
			if err != nil {
				return out, err
			}
			f.Owned = owned
			if b.Present {
				f.LastVerifiedHash = b.Hash
			}
		}
		out.Files = append(out.Files, f)
		if p == g.root {
			out.Routes = append(out.Routes, rootSites(f)...)
			continue
		}
		if path.Dir(p) != g.routes || path.Ext(p) != ".caddy" {
			continue
		}
		s, err := ParseSite(f.Content)
		if err != nil {
			out.Routes = append(out.Routes, dto.Route{ID: f.Path, Name: path.Base(p), File: f.Path, Class: "text-only", Eligibility: refused(dto.UnsupportedFormat, f.Path)})
			continue
		}
		route := dto.Route{ID: f.Path, Name: s.Name, File: f.Path, Class: "text-only", Rule: s.Domain, Upstreams: s.Upstreams, References: []dto.Reference{}, EntryPoints: []string{}, Dependants: []string{}, Eligibility: refused(dto.SourceReadOnly, f.Path)}
		if f.Owned {
			route.Class = "editable"
			route.Eligibility = g.inventory.Eligibility
		}
		out.Routes = append(out.Routes, route)
	}
	return out, nil
}
func rootSites(f dto.File) []dto.Route {
	out := []dto.Route{}
	ds, _ := scan(f.Content)
	for _, d := range ds {
		opening := slices.Index(d.words, "{")
		if d.depth == 0 && opening > 0 && !strings.HasPrefix(d.words[0], "(") {
			out = append(out, dto.Route{ID: f.Path + ":" + d.words[0], Name: strings.TrimSuffix(d.words[0], ","), File: f.Path, Rule: strings.Join(d.words[:opening], " "), Class: "text-only", Eligibility: refused(dto.SourceReadOnly, f.Path), Upstreams: []string{}, References: []dto.Reference{}, EntryPoints: []string{}, Dependants: []string{}})
		}
	}
	return out
}
func (e *engine) Prepare(ctx context.Context, r dto.ApplyRequest) (proxy.Intent, error) {
	i := proxy.Intent{Request: r, Mode: 0644, GraphHashes: map[string]string{}}
	if !hostio.Relative(r.Target) || hostio.Secret(r.Target) {
		return i, problem(dto.InvalidInput, r.Target, 0, "invalid target")
	}
	g, err := e.load(ctx, r.Instance)
	if err != nil {
		return i, err
	}
	if !g.inventory.Eligibility.Writable {
		return i, problem(g.inventory.Eligibility.Reason, g.inventory.Eligibility.Path, 0, "configuration is read-only")
	}
	if g.inventory.AdminOff && !r.Acknowledgements.Restart {
		return i, problem(dto.AcknowledgementRequired, g.root, 0, "container restart required")
	}
	if g.inventory.Resume {
		if g.inventory.AdminOff {
			return i, problem(dto.AdminUnavailable, g.root, 0, "resume comparison requires admin readback")
		}
		diverged, err := e.diverged(ctx, r.Instance, g.inventory)
		if err != nil {
			return i, err
		}
		if diverged && !r.Acknowledgements.ReplaceRunningConfig {
			return i, problem(dto.AcknowledgementRequired, g.root, 0, "running configuration diverged")
		}
	}
	for _, f := range g.files {
		i.GraphHashes[f.Path] = f.Hash
	}
	for _, f := range g.opaque {
		i.GraphHashes[f.Path] = "opaque"
	}
	target := absolute(r.Instance, r.Target)
	f, exists := g.files[target]
	i.Old = proxy.Snapshot([]byte(f.Content), exists)
	if exists {
		i.Mode = f.Mode
	}
	s := state{Root: g.root, Admin: g.inventory.AdminAddress, AdminOff: g.inventory.AdminOff}
	if r.Change.Kind == "insert-import" {
		c, err := e.runtime.InspectContainer(ctx, r.Instance)
		if err != nil {
			return i, err
		}
		if target != g.root || !e.rootWritable(ctx, r.Instance, c, g.root) {
			return i, problem(dto.SingleFileMount, r.Target, 0, "root is outside the writable source or mounted as a file")
		}
		if g.inventory.ImportPresent {
			return i, problem(dto.InvalidInput, r.Target, 0, "import already present")
		}
		text := f.Content
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += g.inventory.ImportLine + "\n"
		i.New = proxy.Snapshot([]byte(text), true)
	} else {
		if !g.inventory.ImportPresent {
			return i, problem(dto.ImportMissing, g.root, 0, g.inventory.ImportLine)
		}
		if path.Dir(target) != g.routes || path.Ext(target) != ".caddy" || target == g.root {
			return i, problem(dto.InvalidInput, r.Target, 0, "one route file in the import directory is required")
		}
		if exists {
			_, owned, err := e.runtime.LastVerified(ctx, r.Instance, r.Target)
			if err != nil {
				return i, err
			}
			if !owned {
				return i, problem(dto.SourceReadOnly, r.Target, 0, "foreign route file")
			}
			old, err := ParseSite(f.Content)
			if err != nil {
				return i, problem(dto.UnsupportedFormat, r.Target, 0, err.Error())
			}
			s.OldSite = &old
		}
		switch r.Change.Kind {
		case "create-route":
			if exists {
				return i, problem(dto.InvalidInput, r.Target, 0, "route already exists")
			}
		case "edit-route", "delete-route":
			if !exists || r.Change.RouteID != r.Target {
				return i, problem(dto.InvalidInput, r.Target, 0, "route identity does not match target")
			}
		default:
			return i, problem(dto.InvalidInput, r.Target, 0, "unsupported change")
		}
		if r.Change.Kind == "delete-route" {
			i.New = proxy.Snapshot(nil, false)
		} else {
			candidate, err := ParseSite(r.Change.Advanced)
			if err != nil {
				return i, problem(dto.InvalidInput, r.Target, 0, err.Error())
			}
			s.NewSite = &candidate
			s.Rename = s.OldSite != nil && renameOnly(*s.OldSite, candidate)
			for p, other := range g.files {
				if p != target {
					if site, err := ParseSite(other.Content); err == nil && site.Name == candidate.Name {
						return i, problem(dto.InvalidInput, r.Target, 0, "duplicate route name")
					}
				}
			}
			i.New = proxy.Snapshot([]byte(r.Change.Advanced), true)
		}
		probeSite := s.NewSite
		if probeSite == nil {
			probeSite = s.OldSite
		}
		s.Before, err = e.probe(ctx, r.Instance, *probeSite)
		if err != nil {
			s.ProbeReason = reason(err, dto.NotObservable)
		}
	}
	i.Reload = !g.inventory.AdminOff
	i.Restart = g.inventory.AdminOff
	i.Engine, err = json.Marshal(s)
	return i, err
}
func (e *engine) Reload(ctx context.Context, i proxy.Intent) error {
	s, err := decode(i)
	if err != nil {
		return err
	}
	res, err := e.runtime.Exec(ctx, i.Request.Instance, "reload", s.Root, "", "")
	err = commandError(res, err, s.Root)
	if err == nil {
		e.activated.Store(i.Request.ID, e.now())
	}
	return err
}
func (e *engine) Restart(ctx context.Context, i proxy.Intent) error {
	_, err := e.runtime.RestartContainer(ctx, i)
	if err == nil {
		e.activated.Store(i.Request.ID, e.now())
	}
	return err
}
func (e *engine) Mark(context.Context, proxy.Intent) error { return dto.Fail(dto.InvalidInput, 422) }

// The shared runtime owns every temporary and staging name, including recovery.
func (e *engine) Cleanup(_ context.Context, i proxy.Intent) error {
	e.activated.Delete(i.Request.ID)
	return nil
}
