// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.yaml.in/yaml/v4"
)

type graph struct {
	result  dto.ReadResult
	docs    map[string]*document
	newest  time.Time
	readErr error // a failed static read; writes return it (see staticInventory)
}

func (e *engine) graph(ctx context.Context, i dto.Instance) (graph, error) {
	g := graph{docs: map[string]*document{}}
	inv, err := e.inventory(ctx, i)
	g.result = dto.ReadResult{Inventory: inv.Inventory, Eligibility: inv.Eligibility, Files: []dto.File{}, OpaqueFiles: []dto.OpaqueFile{}, Routes: []dto.Route{}}
	g.readErr = inv.readErr
	if err != nil {
		return g, err
	}
	dir := routeDirectory(i)
	single := inv.single
	if single {
		dir = inv.FileProvider
	}
	if !path.IsAbs(dir) || !within(i.Source.MountTarget, dir) {
		return g, dto.Fail(dto.InvalidInput, 422)
	}
	files, opaque, err := e.run.CopyHeaders(ctx, i, dir, single, func(h hostio.FileHeader) error {
		if consumed(h.Path) || hostio.Secret(h.Path) {
			if h.ModifiedAt.After(g.newest) {
				g.newest = h.ModifiedAt
			}
		}
		return nil
	})
	if err != nil {
		return g, err
	}
	containerDir := dir
	if single {
		containerDir = path.Dir(dir)
	}
	prefix := strings.TrimPrefix(strings.TrimPrefix(containerDir, i.Source.MountTarget), "/")
	if single && dir == i.Source.MountTarget {
		prefix = ""
	}
	for _, o := range opaque {
		if single {
			o.Path = path.Base(o.Path)
		}
		o.Path = path.Join(prefix, o.Path)
		g.result.OpaqueFiles = append(g.result.OpaqueFiles, o)
	}
	for _, f := range files {
		if !consumed(f.Path) {
			continue
		}
		f.Path = path.Join(prefix, f.Path)
		last, owned, err := e.run.LastVerified(ctx, i, f.Path)
		if err != nil {
			return g, err
		}
		f.Owned = owned
		if last.Present {
			f.LastVerifiedHash = last.Hash
		}
		g.result.Files = append(g.result.Files, f)
		if strings.EqualFold(path.Ext(f.Path), ".toml") {
			denyGraph(&g, dto.UnsupportedFormat, f.Path, 1, "TOML input")
			continue
		}
		if strings.Contains(f.Content, "{{") {
			denyGraph(&g, dto.TemplateInput, f.Path, 1, "template input")
			continue
		}
		d, err := parseDocument(f.Content, f.Path)
		if err != nil {
			var line int = 1
			detail := err.Error()
			if p, ok := err.(*dto.Problem); ok {
				line = p.Line
				detail = p.Detail
			}
			denyGraph(&g, reason(err, dto.InvalidGraph), f.Path, line, detail)
			continue
		}
		g.docs[f.Path] = d
		if err = validateDocument(d, f.Path); err != nil {
			p := err.(*dto.Problem)
			denyGraph(&g, p.Code, f.Path, p.Line, p.Detail)
		}
	}
	sort.Slice(g.result.Files, func(a, b int) bool { return g.result.Files[a].Path < g.result.Files[b].Path })
	sort.Slice(g.result.OpaqueFiles, func(a, b int) bool { return g.result.OpaqueFiles[a].Path < g.result.OpaqueFiles[b].Path })
	if err := validateDefinitions(g.docs); err != nil {
		p := err.(*dto.Problem)
		denyGraph(&g, p.Code, p.Path, p.Line, p.Detail)
	}
	g.result.Routes = classify(g.docs, g.result.OpaqueFiles, inv.Inventory)
	for _, p := range sortedPaths(g.docs) {
		for _, key := range keys(at(g.docs[p].root, "http", "middlewares")) {
			g.result.Middlewares = append(g.result.Middlewares, dto.Reference{Name: key, Kind: "local", File: p})
		}
	}
	for j, r := range g.result.Routes {
		if r.File != "" && !watched(i, r.File) && r.Eligibility.Writable {
			g.result.Routes[j].Class = "text-only"
			g.result.Routes[j].Eligibility = dto.Eligibility{TextOnly: true, Reason: dto.WatchOff, Path: r.File}
		}
	}
	return g, nil
}
func denyGraph(g *graph, code dto.Reason, p string, line int, detail string) {
	if g.result.Eligibility.Writable {
		g.result.Eligibility = dto.Eligibility{Reason: code, Path: p, Line: line, Detail: detail}
	}
}

// watched reports whether Traefik's file watcher sees p: it watches the
// provider directory and its top-level files, not subdirectories.
func watched(i dto.Instance, p string) bool {
	return path.Dir(path.Join(i.Source.MountTarget, p)) == routeDirectory(i)
}

// consumed follows Traefik's directory loader: every file and subdirectory,
// hidden ones included, filtered only by extension.
func consumed(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".yml", ".yaml", ".toml":
		return true
	}
	return false
}
func (e *engine) Read(ctx context.Context, i dto.Instance) (dto.ReadResult, error) {
	g, err := e.graph(ctx, i)
	if err != nil {
		return g.result, err
	}
	if e.containers != nil && contains(g.result.Inventory.Providers, "docker") {
		rows, err := e.containers(ctx)
		if err != nil {
			return g.result, err
		}
		for _, c := range rows {
			names := map[string]bool{}
			for key := range c.Labels {
				bits := strings.Split(key, ".")
				if len(bits) >= 5 && strings.Join(bits[:3], ".") == "traefik.http.routers" {
					names[bits[3]] = true
				}
			}
			for _, name := range sorted(names) {
				prefix := "traefik.http.routers." + name + "."
				route := dto.Route{ID: "docker:" + c.ID + ":" + name, Name: name, Class: "label-read-only", Rule: c.Labels[prefix+"rule"], Service: c.Labels[prefix+"service"], EntryPoints: strings.Split(c.Labels[prefix+"entrypoints"], ",")}
				if len(c.Names) > 0 {
					route.ContainerName = strings.TrimPrefix(c.Names[0], "/")
				}
				if e.project != nil {
					route.ProjectID, err = e.project(ctx, c.Labels["com.docker.compose.project"])
					if err != nil {
						return g.result, err
					}
				}
				g.result.Routes = append(g.result.Routes, route)
			}
		}
	}
	if g.readErr != nil {
		return g.result, &proxy.IncompleteRead{Err: g.readErr}
	}
	return g.result, nil
}
func fileName(s string) string {
	if strings.HasSuffix(s, "@file") {
		return strings.TrimSuffix(s, "@file")
	}
	if strings.Contains(s, "@") {
		return ""
	}
	return s
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

type definition struct {
	file string
	node *yaml.Node
}

func definitions(docs map[string]*document, section, name string) []definition {
	key := fileName(name)
	out := []definition{}
	if key == "" {
		return out
	}
	for p, d := range docs {
		if n := at(d.root, "http", section, key); n != nil {
			out = append(out, definition{p, n})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].file < out[b].file })
	return out
}
func resolve(docs map[string]*document, opaque []dto.OpaqueFile, inv dto.Inventory, section, name string) dto.Reference {
	r := dto.Reference{Name: name, Kind: "unresolved"}
	defs := definitions(docs, section, name)
	if strings.HasSuffix(name, "@internal") {
		r.Kind = "internal"
		return r
	}
	if suffix := strings.LastIndex(name, "@"); suffix >= 0 && name[suffix+1:] != "file" && contains(inv.Providers, name[suffix+1:]) {
		r.Kind = "external"
		return r
	}
	if len(defs) == 1 {
		r.Kind = "local"
		r.File = defs[0].file
		if field(defs[0].node, "plugin") != nil {
			r.Kind = "plugin"
		}
		return r
	}
	if len(defs) == 0 && len(opaque) > 0 && fileName(name) != "" {
		r.Kind = "opaque"
		if len(opaque) == 1 {
			r.File = opaque[0].Path
		}
	}
	return r
}
func sortedPaths(docs map[string]*document) []string {
	paths := []string{}
	for p := range docs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
func classify(docs map[string]*document, opaque []dto.OpaqueFile, inv dto.Inventory) []dto.Route {
	routes := []dto.Route{}
	paths := sortedPaths(docs)
	for _, p := range paths {
		routers := at(docs[p].root, "http", "routers")
		for _, key := range keys(routers) {
			n := field(routers, key)
			r := dto.Route{ID: p + "#" + key, Name: key, File: p, Class: "editable", Eligibility: dto.Eligibility{Writable: true}, Rule: str(field(n, "rule")), Service: str(field(n, "service")), EntryPoints: list(field(n, "entryPoints")), Upstreams: []string{}, References: []dto.Reference{}, Dependants: []string{}}
			if pr := field(n, "priority"); pr != nil {
				_ = pr.Decode(&r.Priority)
			}
			refuse := func(code dto.Reason) {
				r.Class = "text-only"
				r.Eligibility = dto.Eligibility{TextOnly: true, Reason: code}
			}
			for _, name := range keys(n) {
				if !contains([]string{"rule", "entryPoints", "middlewares", "service", "tls", "priority", "ruleSyntax"}, name) {
					refuse(dto.UnsupportedFormat)
				}
			}
			if str(field(n, "ruleSyntax")) == "v2" {
				refuse(dto.RuleSyntaxV2)
			}
			if _, err := parseRule(r.Rule); err != nil {
				refuse(dto.InvalidGraph)
			}
			defs := definitions(docs, "services", r.Service)
			ref := resolve(docs, opaque, inv, "services", r.Service)
			if len(defs) == 1 {
				r.ServiceFile = defs[0].file
				lb := field(defs[0].node, "loadBalancer")
				servers := field(lb, "servers")
				if !onlyKeys(defs[0].node, "loadBalancer") || !onlyKeys(lb, "servers") || servers == nil || servers.Kind != yaml.SequenceNode || len(servers.Content) == 0 {
					refuse(dto.UnsupportedFormat)
				} else {
					for _, server := range servers.Content {
						if !onlyKeys(server, "url") || !validURL(str(field(server, "url"))) {
							refuse(dto.UnsupportedFormat)
						}
						r.Upstreams = append(r.Upstreams, str(field(server, "url")))
					}
				}
			} else if ref.Kind == "opaque" {
				r.ServiceFile = ref.File
			} else {
				refuse(dto.UnsupportedFormat)
			}
			for _, otherPath := range paths {
				for _, otherKey := range keys(at(docs[otherPath].root, "http", "routers")) {
					other := at(docs[otherPath].root, "http", "routers", otherKey)
					if fileName(str(field(other, "service"))) == fileName(r.Service) && fileName(r.Service) != "" {
						r.Dependants = append(r.Dependants, otherPath+"#"+otherKey)
						if otherPath != p {
							refuse(dto.SharedAcrossFiles)
						}
					}
				}
			}
			if len(defs) == 1 && defs[0].file != p {
				refuse(dto.SharedAcrossFiles)
			}
			for _, name := range list(field(n, "middlewares")) {
				r.References = append(r.References, resolve(docs, opaque, inv, "middlewares", name))
			}
			tls := field(n, "tls")
			r.TLS, r.Resolver = tls != nil && tls.Tag != "!!null", str(field(tls, "certResolver"))
			if tls != nil && !onlyKeys(tls, "certResolver", "domains") {
				refuse(dto.UnsupportedFormat)
			}
			catchAll := contains([]string{"PathPrefix(`/`)", "PathPrefix(\"/\")", "HostRegexp(`.*`)", "HostRegexp(\".*\")", "HostRegexp(`^.*$`)", "HostRegexp(\"^.*$\")", "Host(`*`)", "Host(\"*\")"}, canonicalRule(r.Rule))
			redirect := false
			for _, name := range list(field(n, "middlewares")) {
				for _, def := range definitions(docs, "middlewares", name) {
					if field(def.node, "redirectScheme") != nil || field(def.node, "redirectRegex") != nil {
						redirect = true
					}
				}
			}
			if catchAll || redirect {
				r.Class = "infrastructure"
				r.Eligibility = dto.Eligibility{TextOnly: true, Reason: dto.UnsupportedFormat}
			}
			routes = append(routes, r)
		}
	}
	return routes
}
func onlyKeys(n *yaml.Node, allowed ...string) bool {
	if n == nil || n.Kind != yaml.MappingNode {
		return false
	}
	for _, k := range keys(n) {
		if !contains(allowed, k) {
			return false
		}
	}
	return true
}
