// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

type SourceResolver struct {
	githubBase        *url.URL
	registry          *registry.ContainerRegistryService
	transport         http.RoundTripper
	total, perRequest time.Duration
}

func NewResolver(registries *registry.ContainerRegistryService, githubBase string) (*SourceResolver, error) {
	base, err := config.StackImportBaseURL(githubBase)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &SourceResolver{githubBase: base, registry: registries, transport: transport, total: totalBudget, perRequest: requestBudget}, nil
}

func (r *SourceResolver) Resolve(ctx context.Context, req dto.Request) (dto.Result, error) {
	if !utf8.ValidString(req.Ref) || utf8.RuneCountInString(req.Ref) > 512 || strings.TrimSpace(req.Ref) == "" {
		return dto.Result{}, dto.Failure("import.invalid-reference", 400)
	}
	ctx, cancel := context.WithTimeout(ctx, r.total)
	defer cancel()
	budget := &budgetTransport{next: r.transport, perRequest: r.perRequest}
	switch req.Kind {
	case "github":
		return r.github(ctx, req.Ref, budget)
	case "registry":
		if req.Platform == nil {
			req.Platform = &dto.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}
		}
		return r.image(ctx, req, budget)
	default:
		return dto.Result{}, dto.Failure("import.invalid-reference", 400)
	}
}

func (r *SourceResolver) github(ctx context.Context, raw string, budget *budgetTransport) (dto.Result, error) {
	ref, err := parseGitHubRef(raw)
	if err != nil {
		return dto.Result{}, err
	}
	client := &http.Client{Transport: budget, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != r.githubBase.Scheme || req.URL.Host != r.githubBase.Host || req.URL.User != nil {
			return dto.Failure("import.redirect-refused", 502)
		}
		return nil
	}}
	fetch := func(file string) ([]byte, bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.githubBase.String()+"/"+ref.relative(file), nil)
		if err != nil {
			return nil, false, importFailure(err)
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, false, importFailure(err)
		}
		defer res.Body.Close()
		if res.StatusCode == 404 {
			return nil, false, nil
		}
		if res.StatusCode != 200 {
			return nil, false, dto.Failure("import.source-unavailable", 502)
		}
		if res.ContentLength > fileLimit {
			return nil, false, dto.Failure("import.too-large", 413)
		}
		data, err := readFile(res.Body)
		return data, true, err
	}
	var result dto.Result
	found := false
	for _, file := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		data, ok, e := fetch(file)
		if e != nil {
			return result, e
		}
		if !ok {
			continue
		}
		result, err = parseProposal(ctx, data)
		if err != nil {
			return dto.Result{}, err
		}
		found = true
		break
	}
	if !found {
		return dto.Result{}, dto.Failure("import.not-found", 404)
	}
	env, ok, err := fetch(".env.example")
	if err != nil {
		return dto.Result{}, err
	}
	if ok {
		result.EnvNames = uniqueSorted(append(result.EnvNames, envExampleNames(env)...))
		result.Found = append(result.Found, "import.found.env-example")
	}
	result.Title = ref.repo
	result.Source = dto.Source{Kind: "github", CanonicalRef: ref.canonical()}
	return result, nil
}
