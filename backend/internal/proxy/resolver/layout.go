// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	projectdto "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.yaml.in/yaml/v4"
)

type Splice func(string, []string, map[string]any) (string, error)
type LayoutError struct{ Reason dto.UnsupportedLayoutReason }

func (e *LayoutError) Error() string             { return string(dto.UnsupportedLayout) }
func layout(r dto.UnsupportedLayoutReason) error { return &LayoutError{r} }
func field(n *yaml.Node, k string) *yaml.Node {
	if n != nil && n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			if n.Content[i].Value == k {
				return n.Content[i+1]
			}
		}
	}
	return nil
}
func node(b []byte) (*yaml.Node, error) {
	var n yaml.Node
	if yaml.Unmarshal(b, &n) != nil || len(n.Content) != 1 || n.Content[0].Kind != yaml.MappingNode {
		return nil, dto.Fail(dto.StaticParseFailed, 422)
	}
	var walk func(*yaml.Node) error
	walk = func(n *yaml.Node) error {
		if n.Anchor != "" || n.Kind == yaml.AliasNode {
			return layout(dto.ComposeAnchor)
		}
		for _, c := range n.Content {
			if c.Value == "<<" && c.Tag == "!!merge" {
				return layout(dto.ComposeAnchor)
			}
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(n.Content[0]); e != nil {
		return nil, e
	}
	return n.Content[0], nil
}
func ca(c dto.ResolverCA) string {
	switch c.Kind {
	case "letsencrypt-staging":
		return "https://acme-staging-v02.api.letsencrypt.org/directory"
	case "custom":
		return c.URL
	}
	return "https://acme-v02.api.letsencrypt.org/directory"
}
func config(q dto.ResolverRequest) map[string]any {
	acme := map[string]any{"email": q.Email, "storage": q.Storage, "caServer": ca(q.CA)}
	if q.Challenge.Kind == "http" {
		acme["httpChallenge"] = map[string]any{"entryPoint": q.Challenge.EntryPoint}
	} else {
		dns := map[string]any{"provider": q.Challenge.Provider}
		if q.Challenge.DelayBeforeCheck > 0 {
			dns["delayBeforeCheck"] = q.Challenge.DelayBeforeCheck
		}
		if len(q.Challenge.Resolvers) > 0 {
			dns["resolvers"] = q.Challenge.Resolvers
		}
		acme["dnsChallenge"] = dns
	}
	return map[string]any{"acme": acme}
}
func flags(q dto.ResolverRequest) []string {
	p := "--certificatesresolvers." + q.Name + ".acme."
	out := []string{p + "email=" + q.Email, p + "storage=" + q.Storage, p + "caserver=" + ca(q.CA)}
	if q.Challenge.Kind == "http" {
		return append(out, p+"httpchallenge.entrypoint="+q.Challenge.EntryPoint)
	}
	out = append(out, p+"dnschallenge.provider="+string(q.Challenge.Provider))
	if q.Challenge.DelayBeforeCheck > 0 {
		out = append(out, p+"dnschallenge.delaybeforecheck="+itoa(q.Challenge.DelayBeforeCheck))
	}
	if len(q.Challenge.Resolvers) > 0 {
		out = append(out, p+"dnschallenge.resolvers="+strings.Join(q.Challenge.Resolvers, ","))
	}
	return out
}

// Build uses the deploy loader for the candidate effective model; raw bytes only
// live in File.Before/After and never enter the durable envelope or receipt.
func Build(ctx context.Context, t Target, q dto.ResolverRequest, inv dto.Inventory, splice Splice) (Target, error) {
	if slices.Contains(inv.Resolvers, q.Name) {
		return t, dto.Fail(dto.OperationConflict, 409)
	}
	if q.Challenge.Kind == "http" {
		found := false
		for _, e := range inv.EntryPoints {
			if e.Name == q.Challenge.EntryPoint && !e.TLS {
				found = true
			}
		}
		if !found {
			return t, dto.Fail(dto.InvalidInput, 422)
		}
	}
	var cf, ef *File
	for i := range t.Files {
		f := &t.Files[i]
		if f.Kind == "compose" {
			cf = f
		}
		if f.Kind == "env" {
			ef = f
		}
	}
	if cf == nil || ef == nil {
		return t, dto.Fail(dto.TargetUnclear, 409)
	}
	if err := ValidateRaw(cf.Before, t.Service); err != nil {
		return t, err
	}
	root, err := node(cf.Before)
	if err != nil {
		return t, err
	}
	if field(root, "include") != nil {
		return t, layout(dto.ComposeInclude)
	}
	svc := field(field(root, "services"), t.Service)
	if svc == nil {
		return t, dto.Fail(dto.TargetUnclear, 409)
	}
	if field(svc, "extends") != nil {
		return t, layout(dto.ServiceExtends)
	}
	if field(svc, "entrypoint") != nil {
		var ep []string
		_ = field(svc, "entrypoint").Decode(&ep)
		for _, v := range ep {
			if strings.HasPrefix(v, "--") {
				return t, layout(dto.EntrypointFlags)
			}
		}
	}
	if env := field(svc, "environment"); env != nil {
		var raw any
		_ = env.Decode(&raw)
		switch v := raw.(type) {
		case map[string]any:
			for k := range v {
				if strings.HasPrefix(k, "TRAEFIK_") {
					return t, layout(dto.EnvironmentStatic)
				}
			}
		case []any:
			for _, v := range v {
				if s, ok := v.(string); ok && strings.HasPrefix(s, "TRAEFIK_") {
					return t, layout(dto.EnvironmentStatic)
				}
			}
		}
	}
	env, expected, err := Environment(ef.Before, q.Variables)
	if err != nil {
		return t, err
	}
	ef.After = env
	// A changed .env holds secrets and is written owner-only (D-A1). Docklet creates
	// it with FILE_PERM (0644); Mode keeps that, so a rollback restores it.
	if !bytes.Equal(ef.After, ef.Before) {
		ef.WriteMode = 0600
	}
	changes := map[string]any{}
	en := field(svc, "environment")
	if len(q.Variables) > 0 {
		if en != nil && en.Kind == yaml.SequenceNode {
			items := []string{}
			if en.Decode(&items) != nil {
				return t, dto.Fail(dto.EnvConflict, 422)
			}
			for _, v := range q.Variables {
				found := false
				for _, item := range items {
					k, val, has := strings.Cut(item, "=")
					if k == v.Name {
						if found || !has || val != "${"+v.Name+"}" {
							return t, dto.Fail(dto.EnvConflict, 422)
						}
						found = true
					}
				}
				if !found {
					items = append(items, v.Name+"=${"+v.Name+"}")
				}
			}
			changes["environment"] = items
		} else {
			values := map[string]any{}
			if en != nil && en.Decode(&values) != nil {
				return t, dto.Fail(dto.EnvConflict, 422)
			}
			for _, v := range q.Variables {
				if old, exists := values[v.Name]; exists && old != "${"+v.Name+"}" {
					return t, dto.Fail(dto.EnvConflict, 422)
				}
				values[v.Name] = "${" + v.Name + "}"
			}
			changes["environment"] = values
		}
	}
	if t.Static == "" {
		cmd := field(svc, "command")
		if cmd == nil || cmd.Kind != yaml.SequenceNode || cmd.Style&yaml.FlowStyle != 0 {
			return t, layout(dto.ScalarCommand)
		}
		args := []string{}
		if cmd.Decode(&args) != nil {
			return t, layout(dto.ScalarCommand)
		}
		for _, a := range args {
			if a == "" || (!strings.HasPrefix(a, "--") && strings.ContainsAny(a, "\n\r")) {
				return t, layout(dto.ScalarCommand)
			}
		}
		changes["command"] = append(args, flags(q)...)
	} else {
		if strings.EqualFold(path.Ext(t.Static), ".toml") {
			return t, layout(dto.TOMLStatic)
		}
		found := false
		for i := range t.Files {
			f := &t.Files[i]
			if f.Kind != "static" {
				continue
			}
			found = true
			n, e := node(f.Before)
			if e != nil {
				return t, e
			}
			patch := map[string]any{q.Name: config(q)}
			keys := []string{"certificatesResolvers"}
			if existing := field(n, "certificatesResolvers"); existing == nil {
				patch = map[string]any{"certificatesResolvers": patch}
				keys = nil
			} else if field(existing, q.Name) != nil {
				return t, dto.Fail(dto.OperationConflict, 409)
			}
			next, e := splice(string(f.Before), keys, patch)
			if e != nil {
				return t, dto.Fail(dto.StaticParseFailed, 422)
			}
			f.After = []byte(next)
		}
		if !found {
			return t, layout(dto.StaticOutsideProject)
		}
	}
	next, err := splice(string(cf.Before), []string{"services", t.Service}, changes)
	if err != nil {
		return t, dto.Fail(dto.StaticParseFailed, 422)
	}
	cf.After = []byte(next)
	model, err := projects.LoadComposeProjectFromContent(ctx, projectdto.ComposeContentOptions{ComposeContent: next, EnvContent: string(env), WorkingDir: t.Directory, ProjectName: t.Project})
	if err != nil {
		return t, dto.Fail(dto.ComposeLoadFailed, 422)
	}
	if len(projects.NamespaceDependents(model, []string{t.Service})) > 0 {
		return t, layout(dto.NamespaceDependent)
	}
	service, ok := model.Services[t.Service]
	if !ok {
		return t, dto.Fail(dto.ComposeLoadFailed, 422)
	}
	for _, e := range service.EnvFiles {
		if path.Clean(e.Path) != path.Join(t.Directory, ".env") {
			return t, dto.Fail(dto.EnvConflict, 422)
		}
	}
	for k, v := range expected {
		actual := service.Environment[k]
		if actual == nil || *actual != v {
			return t, dto.Fail(dto.EnvConflict, 422)
		}
	}
	if t.Static == "" && !slices.Contains(service.Command, "--certificatesresolvers."+q.Name+".acme.storage="+q.Storage) {
		return t, dto.Fail(dto.ComposeLoadFailed, 422)
	}
	for i := range t.Files {
		f := &t.Files[i]
		if f.After == nil {
			f.After = append([]byte{}, f.Before...)
		}
		f.New = snapshot(f.After, f.Old.Present || f.Kind != "env" || len(f.After) > 0)
	}
	return t, nil
}

// Scoped removes every dependency and build path before the exported compose
// command sees the model. Missing images fail rather than being pulled/built.
func Scoped(model *compose.Project, service string) (*compose.Project, error) {
	selected, err := projects.SelectServices(model, []string{service})
	if err != nil {
		return nil, err
	}
	svc, ok := selected.Services[service]
	if !ok {
		return nil, dto.Fail(dto.TargetUnclear, 409)
	}
	svc.Build = nil
	svc.PullPolicy = compose.PullPolicyNever
	svc.DependsOn = nil
	selected.Services = compose.Services{service: svc}
	selected.DisabledServices = nil
	return selected, nil
}

// ValidateRaw runs before the loader can follow include, extends or env_file.
// Restricting external sources also makes the candidate loader equal to deploy.
func ValidateRaw(data []byte, service string) error {
	root, err := node(data)
	if err != nil {
		return err
	}
	if field(root, "include") != nil {
		return layout(dto.ComposeInclude)
	}
	services := field(root, "services")
	if services == nil {
		return dto.Fail(dto.ComposeLoadFailed, 422)
	}
	for j := 1; j < len(services.Content); j += 2 {
		svc := services.Content[j]
		if field(svc, "extends") != nil {
			return layout(dto.ServiceExtends)
		}
		if en := field(svc, "env_file"); en != nil {
			var names []string
			if en.Kind == yaml.ScalarNode {
				names = []string{en.Value}
			} else if en.Decode(&names) != nil {
				return dto.Fail(dto.EnvConflict, 422)
			}
			for _, n := range names {
				if n != ".env" {
					return dto.Fail(dto.EnvConflict, 422)
				}
			}
		}
	}
	if svc := field(services, service); svc != nil {
		if cmd := field(svc, "command"); cmd != nil && (cmd.Kind != yaml.SequenceNode || cmd.Style&yaml.FlowStyle != 0) {
			return layout(dto.ScalarCommand)
		}
	}
	if field(services, service) == nil {
		return dto.Fail(dto.TargetUnclear, 409)
	}
	return nil
}
