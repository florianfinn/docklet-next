// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"context"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/stackimport"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"gopkg.in/yaml.v3"
)

var serviceCharacters = regexp.MustCompile(`[^a-z0-9_-]+`)

func (r *SourceResolver) image(ctx context.Context, req dto.Request, budget *budgetTransport) (dto.Result, error) {
	if r.registry == nil {
		return dto.Result{}, dto.Failure("import.source-unavailable", 502)
	}
	resolved, err := r.registry.ResolveImportImage(ctx, req.Ref, v1.Platform{OS: req.Platform.OS, Architecture: req.Platform.Architecture}, func(next http.RoundTripper) http.RoundTripper { budget.next = next; return budget })
	if err != nil {
		return dto.Result{}, importFailure(err)
	}
	cfg := resolved.Config.Config
	imageRef := req.Ref
	if !strings.Contains(imageRef, "@") && !strings.Contains(path.Base(imageRef), ":") {
		imageRef += ":latest"
	}
	service := map[string]any{"image": imageRef, "restart": "unless-stopped"}
	title := strings.SplitN(path.Base(strings.SplitN(resolved.Reference, "@", 2)[0]), ":", 2)[0]
	title = strings.Trim(serviceCharacters.ReplaceAllString(strings.ToLower(title), "-"), "-_")
	if title == "" {
		title = "app"
	}
	result := dto.Result{Title: title, EnvNames: []string{}, Found: []string{"import.found.image-config"}, Missing: []string{"import.missing.compose"}, Source: dto.Source{Kind: "registry", CanonicalRef: resolved.Reference, ResolvedDigest: resolved.Digest}}
	ports := []string{}
	for p := range cfg.ExposedPorts {
		port, proto, _ := strings.Cut(p, "/")
		if proto == "" {
			proto = "tcp"
		}
		ports = append(ports, port+":"+port+"/"+proto)
	}
	if len(ports) > 0 {
		slices.Sort(ports)
		service["ports"] = ports
		result.Found = append(result.Found, "import.found.ports")
	}
	volumes := map[string]any{}
	mounts := []string{}
	destinations := []string{}
	for destination := range cfg.Volumes {
		destinations = append(destinations, destination)
	}
	slices.Sort(destinations)
	for i, destination := range destinations {
		// Stable ordinals prevent different image paths from colliding after normalization.
		name := title + "-data-" + strconv.Itoa(i+1)
		volumes[name] = map[string]any{}
		mounts = append(mounts, name+":"+composeLiteral(destination))
	}
	if len(mounts) > 0 {
		service["volumes"] = mounts
		result.Found = append(result.Found, "import.found.volumes")
	}
	environment := map[string]any{}
	for _, entry := range cfg.Env {
		key, value, hasDefault := strings.Cut(entry, "=")
		if !envName.MatchString(key) {
			continue
		}
		if hasDefault {
			environment[key] = composeLiteral(value)
		} else {
			environment[key] = nil
			result.EnvNames = append(result.EnvNames, key)
		}
	}
	if len(environment) > 0 {
		service["environment"] = environment
		result.Found = append(result.Found, "import.found.environment")
	}
	model := map[string]any{"services": map[string]any{title: service}}
	if len(volumes) > 0 {
		model["volumes"] = volumes
	}
	data, err := yaml.Marshal(model)
	if err != nil {
		return dto.Result{}, dto.Failure("import.source-unavailable", 502)
	}
	if _, err = parseProposal(ctx, data); err != nil {
		return dto.Result{}, err
	}
	result.ComposeContent = string(data)
	result.EnvNames = uniqueSorted(result.EnvNames)
	return result, nil
}

// Image config strings are literal Docker values, while Compose interpolates dollars.
func composeLiteral(value string) string { return strings.ReplaceAll(value, "$", "$$") }
