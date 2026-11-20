// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"errors"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.yaml.in/yaml/v4"
)

type staticInventory struct {
	dto.Inventory
	single bool
	// readErr keeps a failed static-file read: shown as incomplete inventory, but a
	// write must return it rather than settle on that presentation.
	readErr error
}

func readonly(v *staticInventory, code dto.Reason, p string) {
	if v.Eligibility.Writable {
		v.Eligibility = dto.Eligibility{Reason: code, Path: p}
	}
}
func (e *engine) Inventory(ctx context.Context, i dto.Instance) (dto.Inventory, error) {
	v, err := e.inventory(ctx, i)
	return v.Inventory, err
}

func (e *engine) inventory(ctx context.Context, i dto.Instance) (staticInventory, error) {
	v := staticInventory{Inventory: dto.Inventory{Identity: i.Identity, Generation: i.Generation, Watch: true, ThrottleMillis: 2000, EntryPoints: []dto.EntryPoint{}, Resolvers: []string{}, Providers: []string{}, Plugins: []string{}, Imports: []string{}, Eligibility: dto.Eligibility{Writable: true}}}
	if i.Source.ReadOnly || i.Source.Kind == dto.UnknownSourceKind || i.Source.Kind == dto.SingleFileSource {
		code := i.Source.Reason
		if code == "" {
			code = dto.SourceReadOnly
			if i.Source.Kind == dto.SingleFileSource {
				code = dto.SingleFileMount
			}
			if i.Source.Kind == dto.UnknownSourceKind {
				code = dto.UnknownSource
			}
		}
		readonly(&v, code, "")
	}
	c, err := e.run.InspectContainer(ctx, i)
	if err != nil {
		return v, err
	}
	if c.State != nil {
		v.Identity.StartedAt = c.State.StartedAt
	}
	if c.Config == nil {
		readonly(&v, dto.IncompleteInventory, "")
		return v, nil
	}
	opts := map[string]string{}
	configFile := i.RootFile
	args := append(append([]string{}, c.Config.Entrypoint...), c.Config.Cmd...)
	for j := 0; j < len(args); j++ {
		a := args[j]
		if !strings.HasPrefix(a, "--") {
			continue
		}
		k, val, has := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !has {
			val = "true"
			if j+1 < len(args) && !strings.HasPrefix(args[j+1], "-") {
				j++
				val = args[j]
			}
		}
		k = optionKey(k)
		opts[k] = val
		if k == "configfile" {
			configFile = val
		}
	}
	env := map[string]string{}
	envConfig := ""
	for _, s := range c.Config.Env {
		k, val, ok := strings.Cut(s, "=")
		if ok && strings.HasPrefix(k, "TRAEFIK_") {
			k = strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(k, "TRAEFIK_"), "_", "."))
			env[k] = val
			if k == "configfile" {
				envConfig = val
			}
		}
	}
	// Traefik chooses one static source; mixed source configurations are refused.
	static := map[string]string{}
	candidates := []string{configFile}
	if configFile == "" {
		// Traefik's file finder order (pkg/cli/loader_file.go).
		candidates = []string{"/etc/traefik/traefik.toml", "/etc/traefik/traefik.yaml", "/etc/traefik/traefik.yml"}
	}
	for _, p := range candidates {
		if !path.IsAbs(p) || path.Clean(p) != p {
			readonly(&v, dto.IncompleteInventory, p)
			continue
		}
		files, opaque, copyErr := e.run.Copy(ctx, i, p, true)
		if copyErr != nil {
			if errdefs.IsNotFound(copyErr) && configFile == "" {
				continue
			}
			if !errdefs.IsNotFound(copyErr) && proxy.Transient(copyErr) {
				v.readErr = copyErr
			}
			readonly(&v, dto.IncompleteInventory, p)
			break
		}
		if len(opaque) > 0 || len(files) != 1 {
			readonly(&v, dto.IncompleteInventory, p)
			break
		}
		v.RootFile = p
		if strings.EqualFold(path.Ext(p), ".toml") {
			readonly(&v, dto.UnsupportedFormat, p)
			break
		}
		doc, parseErr := parseDocument(files[0].Content, p)
		if parseErr != nil {
			readonly(&v, dto.IncompleteInventory, p)
			break
		}
		flatten(doc.root, "", static)
		break
	}
	// Traefik reads the configuration file location from flags only; without a
	// found file an environment location is ignored, against the operator's intent.
	if len(static) == 0 && envConfig != "" {
		readonly(&v, dto.IncompleteInventory, envConfig)
	}
	selected := static
	if len(static) == 0 {
		selected = env
		if len(opts) > 0 {
			selected = opts
		}
	}
	if len(static) > 0 && (hasStaticOptions(opts) || hasStaticOptions(env)) {
		readonly(&v, dto.IncompleteInventory, v.RootFile)
	}
	if len(static) == 0 && hasStaticOptions(opts) && hasStaticOptions(env) {
		readonly(&v, dto.IncompleteInventory, "")
	}
	entries := map[string]*dto.EntryPoint{}
	resolvers := map[string]bool{}
	providers := map[string]bool{}
	plugins := map[string]bool{}
	fileName := ""
	directory := ""
	for k, val := range selected {
		bits := strings.Split(k, ".")
		switch {
		case len(bits) >= 3 && bits[0] == "entrypoints":
			ep := entries[bits[1]]
			if ep == nil {
				ep = &dto.EntryPoint{Name: bits[1]}
				entries[bits[1]] = ep
			}
			if strings.Join(bits[2:], ".") == "address" {
				ep.Address = val
			}
			if strings.HasPrefix(strings.Join(bits[2:], "."), "http.tls") {
				ep.TLS = val != "false"
			}
		case len(bits) >= 3 && bits[0] == "certificatesresolvers":
			resolvers[bits[1]] = true
		case len(bits) >= 2 && bits[0] == "providers":
			if bits[1] == "providersthrottleduration" {
				d, err := time.ParseDuration(val)
				// Traefik durations accept a plain integer as seconds.
				if seconds, intErr := strconv.ParseInt(val, 10, 64); intErr == nil && seconds >= 0 && seconds < 1<<31 {
					d, err = time.Duration(seconds)*time.Second, nil
				}
				if err != nil || d < 0 || d > (time.Duration(1<<63-1)-10*time.Second)/2 {
					readonly(&v, dto.IncompleteInventory, "")
				} else {
					v.ThrottleMillis = d.Milliseconds()
				}
				continue
			}
			if bits[1] == "file" {
				providers["file"] = true
				if len(bits) == 3 {
					switch bits[2] {
					case "directory":
						directory = val
					case "filename":
						fileName = val
					case "watch":
						b, err := strconv.ParseBool(val)
						if err != nil {
							readonly(&v, dto.IncompleteInventory, "")
						} else {
							v.Watch = b
						}
					}
				}
			}
			if bits[1] == "docker" && val != "false" {
				providers["docker"] = true
			}
		case len(bits) >= 3 && bits[0] == "experimental" && (bits[1] == "plugins" || bits[1] == "localplugins"):
			plugins[bits[2]] = true
		}
	}
	if directory != "" && fileName != "" {
		readonly(&v, dto.IncompleteInventory, "")
	}
	v.FileProvider = directory
	if fileName != "" {
		v.FileProvider = fileName
		v.single = true
		readonly(&v, dto.SingleFileMount, fileName)
	}
	if directory == "" && fileName == "" {
		readonly(&v, dto.IncompleteInventory, "")
	}
	if directory != "" && (!path.IsAbs(directory) || path.Clean(directory) != directory || !within(i.Source.MountTarget, directory) || directory != routeDirectory(i)) {
		readonly(&v, dto.IncompleteInventory, directory)
	}
	if !v.Watch {
		readonly(&v, dto.WatchOff, v.FileProvider)
	}
	for _, ep := range entries {
		_, port, err := net.SplitHostPort(strings.TrimSuffix(strings.TrimSuffix(ep.Address, "/tcp"), "/udp"))
		n, conv := strconv.Atoi(port)
		if err != nil || conv != nil || n < 1 || n > 65535 {
			readonly(&v, dto.IncompleteInventory, "")
		}
		v.EntryPoints = append(v.EntryPoints, *ep)
	}
	if len(v.EntryPoints) == 0 {
		readonly(&v, dto.IncompleteInventory, "")
	}
	sort.Slice(v.EntryPoints, func(a, b int) bool { return v.EntryPoints[a].Name < v.EntryPoints[b].Name })
	v.Resolvers = sorted(resolvers)
	v.ResolverSummaries = resolverSummaries(selected, c.Config.Env)
	v.Providers = sorted(providers)
	v.Plugins = sorted(plugins)
	return v, nil
}
func hasStaticOptions(m map[string]string) bool {
	for k := range m {
		if k != "configfile" {
			return true
		}
	}
	return false
}
func flatten(n *yaml.Node, p string, out map[string]string) {
	if n.Kind == yaml.MappingNode {
		if len(n.Content) == 0 {
			out[optionKey(p)] = "true"
		}
		for j := 0; j < len(n.Content); j += 2 {
			k := n.Content[j].Value
			if p != "" {
				k = p + "." + k
			}
			flatten(n.Content[j+1], k, out)
		}
	} else if n.Kind == yaml.ScalarNode {
		out[optionKey(p)] = n.Value
	}
}
func sorted(m map[string]bool) []string {
	v := []string{}
	for k := range m {
		v = append(v, k)
	}
	sort.Strings(v)
	return v
}
func routeDirectory(i dto.Instance) string {
	if path.IsAbs(i.RouteDirectory) {
		return path.Clean(i.RouteDirectory)
	}
	return path.Join(i.Source.MountTarget, i.RouteDirectory)
}
func within(root, p string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}
func reason(err error, fallback dto.Reason) dto.Reason {
	var p *dto.Problem
	if errors.As(err, &p) {
		return p.Code
	}
	return fallback
}

// Dynamic dictionary names retain their identity; static field names are folded.
func optionKey(s string) string {
	parts := strings.Split(s, ".")
	for j := range parts {
		if j == 1 && (strings.EqualFold(parts[0], "entrypoints") || strings.EqualFold(parts[0], "certificatesresolvers")) {
			continue
		}
		if j == 2 && strings.EqualFold(parts[0], "experimental") && (strings.EqualFold(parts[1], "plugins") || strings.EqualFold(parts[1], "localplugins")) {
			continue
		}
		parts[j] = strings.ToLower(parts[j])
	}
	return strings.Join(parts, ".")
}
