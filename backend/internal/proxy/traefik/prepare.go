// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type probe struct {
	Route, Host, URL string
	Before           hostio.Response
	Reason           dto.Reason
}
type engineState struct {
	Inventory  dto.Inventory
	Probes     []probe
	Unresolved bool
	// Scope names the target's routers, services and middlewares (old and new)
	// and its references; only their log records reject the change.
	Scope       []string
	Marker      probe
	ForceMarker bool
}

func state(in proxy.Intent) (engineState, error) {
	var s engineState
	err := json.Unmarshal(in.Engine, &s)
	return s, err
}
func (e *engine) Prepare(ctx context.Context, r dto.ApplyRequest) (proxy.Intent, error) {
	in := proxy.Intent{Request: r, Mode: 0644, GraphHashes: map[string]string{}}
	if !hostio.Relative(r.Target) || hostio.Secret(r.Target) || !consumed(r.Target) || strings.EqualFold(path.Ext(r.Target), ".toml") || !within(routeDirectory(r.Instance), path.Join(r.Instance.Source.MountTarget, r.Target)) {
		return in, dto.Fail(dto.InvalidInput, 422)
	}
	if !watched(r.Instance, r.Target) {
		return in, problem(dto.WatchOff, r.Target, 0, "Traefik does not watch files in subdirectories")
	}
	g, err := e.graph(ctx, r.Instance)
	if err != nil {
		return in, err
	}
	if g.readErr != nil {
		return in, g.readErr // a failed static read is no eligibility refusal for a write
	}
	var current *dto.File
	for j, f := range g.result.Files {
		in.GraphHashes[f.Path] = f.Hash
		if f.Path == r.Target {
			current = &g.result.Files[j]
		}
	}
	for _, o := range g.result.OpaqueFiles {
		in.GraphHashes[o.Path] = "opaque"
	}
	if current != nil {
		in.Old = proxy.Snapshot([]byte(current.Content), true)
		in.Mode = current.Mode
	} else {
		in.Old = proxy.Snapshot(nil, false)
	}
	if r.Change.Kind == "restore-owned" {
		if !g.result.Inventory.Eligibility.Writable {
			return in, eligibilityError(g.result.Inventory.Eligibility)
		}
		last, owned, err := e.run.LastVerified(ctx, r.Instance, r.Target)
		if err != nil {
			return in, err
		}
		if !owned || !last.Present {
			return in, dto.Fail(dto.InvalidInput, 422)
		}
		in.New = last
	} else {
		if !g.result.Eligibility.Writable {
			return in, eligibilityError(g.result.Eligibility)
		}
		var next string
		if r.Change.Kind == "create-route" {
			if in.Old.Present || !namePattern.MatchString(r.Change.Name) || len(r.Change.Upstreams) == 0 {
				return in, dto.Fail(dto.InvalidInput, 422)
			}
			service := r.Change.Service
			if service == "" {
				service = r.Change.Name
			}
			if !namePattern.MatchString(service) {
				return in, dto.Fail(dto.InvalidInput, 422)
			}
			router := map[string]any{"rule": r.Change.Rule, "service": service}
			setRouterPatch(router, r.Change)
			doc := map[string]any{"http": map[string]any{"routers": map[string]any{r.Change.Name: router}, "services": map[string]any{service: backend(r.Change.Upstreams)}}}
			next, err = render(doc, 0)
			next += "\n"
		} else {
			if current == nil {
				return in, dto.Fail(dto.InvalidInput, 422)
			}
			d := g.docs[r.Target]
			if d == nil {
				return in, dto.Fail(dto.InvalidGraph, 422)
			}
			route, ok := findRoute(g.result.Routes, r.Change.RouteID, r.Target)
			if !ok || route.Class != "editable" {
				return in, dto.Fail(dto.UnsupportedFormat, 422)
			}
			router := at(d.root, "http", "routers", route.Name)
			var edits []edit
			switch r.Change.Kind {
			case "delete-route":
				// Traefik refuses an empty routers mapping ("routers cannot be a standalone
				// element", measured v3.7.16), so removing the last router drops the section,
				// and an emptied http section with it; services and the file stay.
				parent, key := at(d.root, "http", "routers"), route.Name
				if len(parent.Content) == 2 {
					parent, key = at(d.root, "http"), "routers"
					if len(parent.Content) == 2 {
						parent, key = d.root, "http"
					}
				}
				for j := 0; j < len(parent.Content); j += 2 {
					if parent.Content[j].Value == key {
						var ed edit
						ed, err = d.remove(parent, j)
						edits = append(edits, ed)
					}
				}
			case "edit-route", "edit-service":
				if r.Change.Kind == "edit-route" {
					patch := map[string]any{}
					setRouterPatch(patch, r.Change)
					err = d.patch(router, patch, &edits)
					if r.Change.Name != "" && r.Change.Name != route.Name {
						if !namePattern.MatchString(r.Change.Name) || at(d.root, "http", "routers", r.Change.Name) != nil {
							return in, dto.Fail(dto.InvalidInput, 422)
						}
						routers := at(d.root, "http", "routers")
						for j := 0; j < len(routers.Content); j += 2 {
							if routers.Content[j].Value == route.Name {
								err = d.replace(routers.Content[j], r.Change.Name, &edits)
							}
						}
					}
				}
				if r.Change.Upstreams != nil {
					if r.Change.Service != "" && r.Change.Service != route.Service {
						return in, dto.Fail(dto.InvalidInput, 422)
					}
					if len(r.Change.Upstreams) == 0 || route.ServiceFile != r.Target {
						return in, dto.Fail(dto.UnsupportedFormat, 422)
					}
					err = d.patch(at(d.root, "http", "services", fileName(route.Service), "loadBalancer"), map[string]any{"servers": servers(r.Change.Upstreams)}, &edits)
				}
			default:
				return in, dto.Fail(dto.InvalidInput, 422)
			}
			if err == nil {
				next, err = d.apply(edits)
			}
		}
		if err != nil {
			return in, problem(dto.UnsupportedFormat, r.Target, 1, err.Error())
		}
		in.New = proxy.Snapshot([]byte(next), true)
	}
	d, err := parseDocument(string(in.New.Content), r.Target)
	if err != nil {
		return in, err
	}
	if err = validateDocument(d, r.Target); err != nil {
		return in, err
	}
	nextDocs := map[string]*document{}
	for p, doc := range g.docs {
		if p != r.Target {
			nextDocs[p] = doc
		}
	}
	nextDocs[r.Target] = d
	// A restore may repair only the owned target; every sibling still validates.
	for _, f := range g.result.Files {
		if f.Path != r.Target {
			doc, err := parseDocument(f.Content, f.Path)
			if err != nil {
				return in, err
			}
			if err = validateDocument(doc, f.Path); err != nil {
				return in, err
			}
		}
	}
	if err = validateDefinitions(nextDocs); err != nil {
		return in, err
	}
	if err = checkNewReferences(nextDocs, g.result.OpaqueFiles, g.result.Inventory, in); err != nil {
		return in, err
	}
	s := engineState{Inventory: g.result.Inventory, Probes: []probe{}, ForceMarker: r.Change.Resolver != ""}
	unresolved, err := newUnresolved(nextDocs, g.result.OpaqueFiles, g.result.Inventory, in)
	if err != nil {
		return in, err
	}
	s.Unresolved = len(unresolved) > 0
	s.Scope = logScope(in.Old, d, r.Target)
	// Opaque input has no parseable definitions: the running provider must be healthy.
	if len(g.result.OpaqueFiles) > 0 {
		since := g.newest
		if started, parseErr := time.Parse(time.RFC3339Nano, g.result.Inventory.Identity.StartedAt); parseErr == nil && started.After(since) {
			since = started
		}
		if err = e.checkLogs(ctx, r.Instance, since, nil); err != nil {
			return in, err
		}
	}
	after := classify(nextDocs, g.result.OpaqueFiles, g.result.Inventory)
	selected := []dto.Route{}
	if r.Change.Kind == "restore-owned" || r.Change.Kind == "create-route" {
		for _, route := range after {
			if route.File == r.Target {
				selected = append(selected, route)
			}
		}
	} else {
		old, ok := findRoute(g.result.Routes, r.Change.RouteID, r.Target)
		if !ok {
			return in, dto.Fail(dto.InvalidInput, 422)
		}
		ids := []string{old.ID}
		if r.Change.Upstreams != nil {
			ids = old.Dependants
		}
		for _, id := range ids {
			route, exists := findRoute(after, id, r.Target)
			if !exists {
				route, _ = findRoute(g.result.Routes, id, r.Target)
			}
			if id == old.ID && r.Change.Name != "" {
				if renamed, ok := findRoute(after, r.Target+"#"+r.Change.Name, r.Target); ok {
					route = renamed
				}
			}
			selected = append(selected, route)
		}
	}
	for _, route := range selected {
		inv := s.Inventory
		inv.EntryPoints = append([]dto.EntryPoint{}, inv.EntryPoints...)
		doc := nextDocs[route.File]
		if doc == nil || at(doc.root, "http", "routers", route.Name) == nil {
			doc = g.docs[route.File]
		}
		if doc != nil {
			if at(doc.root, "http", "routers", route.Name, "tls") != nil {
				for j := range inv.EntryPoints {
					if len(route.EntryPoints) == 0 || contains(route.EntryPoints, inv.EntryPoints[j].Name) {
						inv.EntryPoints[j].TLS = true
					}
				}
			}
		}
		probes := routeProbes(route, inv)
		for _, p := range probes {
			if p.Reason == "" {
				p.Before, err = e.probe(ctx, r.Instance, p)
				if err != nil {
					p.Reason = reason(err, dto.NotObservable)
				}
			}
			s.Probes = append(s.Probes, p)
		}
	}
	s.Marker = markerProbe(r.ID, s.Inventory)
	if s.Marker.Reason == "" {
		s.Marker.Before, err = e.probe(ctx, r.Instance, s.Marker)
		if err == nil && s.Marker.Before.Status == 0 {
			s.Marker.Reason = dto.NotObservable
		}
		if err != nil {
			s.Marker.Reason = reason(err, dto.NotObservable)
		}
	}
	in.Engine, err = json.Marshal(s)
	return in, err
}
func servers(urls []string) []any {
	a := []any{}
	for _, u := range urls {
		a = append(a, map[string]any{"url": u})
	}
	return a
}
func backend(urls []string) map[string]any {
	return map[string]any{"loadBalancer": map[string]any{"servers": servers(urls)}}
}
func setRouterPatch(m map[string]any, c dto.Change) {
	if c.Rule != "" {
		m["rule"] = c.Rule
	}
	if c.Service != "" {
		m["service"] = c.Service
	}
	if c.Priority != nil {
		m["priority"] = *c.Priority
	}
	if c.EntryPoints != nil {
		m["entryPoints"] = c.EntryPoints
	}
	if c.Middlewares != nil {
		m["middlewares"] = c.Middlewares
	}
	if c.Resolver != "" {
		m["tls"] = map[string]any{"certResolver": c.Resolver}
	}
}
func findRoute(routes []dto.Route, id, p string) (dto.Route, bool) {
	for _, r := range routes {
		if r.File == p && (r.ID == id || r.Name == id) {
			return r, true
		}
	}
	return dto.Route{}, false
}

type graphRef struct{ owner, section, name string }

func refs(docs map[string]*document) []graphRef {
	out := []graphRef{}
	for p, d := range docs {
		for _, key := range keys(at(d.root, "http", "routers")) {
			r := at(d.root, "http", "routers", key)
			owner := p + "#" + key
			out = append(out, graphRef{owner, "services", str(field(r, "service"))})
			for _, v := range list(field(r, "middlewares")) {
				out = append(out, graphRef{owner, "middlewares", v})
			}
			for _, ep := range list(field(r, "entryPoints")) {
				out = append(out, graphRef{owner, "entryPoints", ep})
			}
			if v := str(at(r, "tls", "certResolver")); v != "" {
				out = append(out, graphRef{owner, "resolvers", v})
			}
			if v := str(at(r, "tls", "options")); v != "" {
				out = append(out, graphRef{owner, "tlsOptions", v})
			}
		}
		for _, key := range keys(at(d.root, "http", "middlewares")) {
			for _, v := range list(at(d.root, "http", "middlewares", key, "chain", "middlewares")) {
				out = append(out, graphRef{p + "#middleware:" + key, "middlewares", v})
			}
		}
		for _, key := range keys(at(d.root, "http", "services")) {
			s := at(d.root, "http", "services", key)
			for _, v := range []string{str(at(s, "mirroring", "service")), str(at(s, "failover", "service")), str(at(s, "failover", "fallback"))} {
				if v != "" {
					out = append(out, graphRef{p + "#service:" + key, "services", v})
				}
			}
			for _, kind := range []string{"weighted", "mirroring"} {
				items := at(s, kind, "services")
				if kind == "mirroring" {
					items = at(s, kind, "mirrors")
				}
				if items != nil {
					for _, item := range items.Content {
						out = append(out, graphRef{p + "#service:" + key, "services", str(field(item, "name"))})
					}
				}
			}
		}
	}
	return out
}
func newUnresolved(docs map[string]*document, opaque []dto.OpaqueFile, inv dto.Inventory, in proxy.Intent) ([]string, error) {
	old := map[graphRef]bool{}
	if in.Old.Present {
		d, err := parseDocument(string(in.Old.Content), in.Request.Target)
		if err == nil {
			for _, ref := range refs(map[string]*document{in.Request.Target: d}) {
				if in.Request.Change.Name != "" && ref.owner == in.Request.Change.RouteID {
					ref.owner = in.Request.Target + "#" + in.Request.Change.Name
				}
				old[ref] = true
			}
		}
	}
	out := []string{}
	for _, ref := range refs(docs) {
		if !strings.HasPrefix(ref.owner, in.Request.Target+"#") || old[ref] {
			continue
		}
		resolved := false
		switch ref.section {
		case "entryPoints":
			for _, ep := range inv.EntryPoints {
				if ep.Name == ref.name {
					resolved = true
				}
			}
		case "resolvers":
			resolved = contains(inv.Resolvers, ref.name)
			if !resolved {
				return nil, problem(dto.InvalidGraph, in.Request.Target, 1, "select an existing resolver")
			}
		case "tlsOptions":
			for _, d := range docs {
				if at(d.root, "tls", "options", fileName(ref.name)) != nil {
					resolved = true
				}
			}
			if ref.name == "default" {
				resolved = true
			}
		default:
			r := resolve(docs, opaque, inv, ref.section, ref.name)
			resolved = r.Kind != "unresolved" && r.Kind != "opaque"
		}
		if !resolved && !contains(out, ref.name) {
			out = append(out, ref.name)
		}
	}
	return out, nil
}
func checkNewReferences(docs map[string]*document, opaque []dto.OpaqueFile, inv dto.Inventory, in proxy.Intent) error {
	names, err := newUnresolved(docs, opaque, inv, in)
	if err != nil {
		return err
	}
	for _, name := range names {
		if !contains(in.Request.Acknowledgements.UnresolvedReferences, name) {
			return problem(dto.AcknowledgementRequired, in.Request.Target, 1, fmt.Sprintf("unresolved reference: %s", name))
		}
	}
	return nil
}

func logScope(old proxy.Bytes, next *document, target string) []string {
	docs := []*document{next}
	if old.Present {
		if d, err := parseDocument(string(old.Content), target); err == nil {
			docs = append(docs, d)
		}
	}
	out := []string{}
	add := func(name string) {
		if name == "" {
			return
		}
		if !strings.Contains(name, "@") {
			name += "@file"
		}
		if !contains(out, name) {
			out = append(out, name)
		}
	}
	for _, d := range docs {
		for _, section := range []string{"routers", "services", "middlewares"} {
			for _, key := range keys(at(d.root, "http", section)) {
				add(key)
			}
		}
		for _, ref := range refs(map[string]*document{target: d}) {
			if ref.section == "services" || ref.section == "middlewares" {
				add(ref.name)
			}
		}
	}
	return out
}
