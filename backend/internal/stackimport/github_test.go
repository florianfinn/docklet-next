// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/stretchr/testify/require"
)

const validCompose = "services:\n  app:\n    image: alpine:3\n"

func requireProblem(t *testing.T, err error, code string, status int) {
	t.Helper()
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, code, problem.Code)
	require.Equal(t, status, problem.Status)
	require.Equal(t, code, err.Error(), "no upstream diagnostic text")
}
func githubResolver(t *testing.T, h http.Handler) (*SourceResolver, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	r, err := NewResolver(nil, server.URL)
	require.NoError(t, err)
	return r, server
}
func importGitHub(r *SourceResolver) (dto.Result, error) {
	return r.Resolve(context.Background(), dto.Request{Kind: "github", Ref: "owner/repo"})
} //nolint:forbidigo // Test helper supplies a root context to the request budget.

func TestGitHubAcceptedReferencesAndCandidates(t *testing.T) {
	for _, test := range []struct{ ref, relative string }{
		{"owner/repo", "owner/repo/HEAD"},
		{"owner/repo/path/to dir", "owner/repo/HEAD/path/to%20dir"},
		{"owner/repo/config:release@1", "owner/repo/HEAD/config:release@1"},
		{"https://github.com/owner/repo", "owner/repo/HEAD"},
		{"https://github.com/owner/repo/tree/release-1/path", "owner/repo/release-1/path"},
	} {
		t.Run(test.ref, func(t *testing.T) {
			requests := []string{}
			r, _ := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests = append(requests, req.URL.EscapedPath())
				switch req.URL.EscapedPath() {
				case "/" + test.relative + "/docker-compose.yml":
					_, _ = io.WriteString(w, validCompose)
				case "/" + test.relative + "/.env.example":
					_, _ = io.WriteString(w, "# COMMENT=secret\nexport USER=private-value\nPASSWORD=example\nUSER=duplicate\nBAD-NAME=no\n")
				default:
					w.WriteHeader(404)
				}
			}))
			result, err := r.Resolve(t.Context(), dto.Request{Kind: "github", Ref: test.ref})
			require.NoError(t, err)
			require.Equal(t, validCompose, result.ComposeContent)
			require.Equal(t, []string{"PASSWORD", "USER"}, result.EnvNames)
			require.Equal(t, []string{"import.found.compose", "import.found.env-example"}, result.Found)
			require.Len(t, requests, 5)
			for i, file := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml", ".env.example"} {
				require.Equal(t, "/"+test.relative+"/"+file, requests[i])
			}
			require.NotContains(t, fmt.Sprint(result), "private-value")
			require.Equal(t, "repo", result.Title)
			require.Equal(t, "github", result.Source.Kind)
		})
	}
}
func TestGitHubInvalidReferencesMakeNoRequests(t *testing.T) {
	var calls atomic.Int32
	r, _ := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { calls.Add(1); _, _ = io.WriteString(w, validCompose) }))
	for _, ref := range []string{"../repo", "owner/..", "owner/repo/..", "owner/repo/%2e%2e", "owner/repo/%252e%252e", "owner/repo//dir", "owner/repo/dir\\evil", "git@github.example.com:owner/repo", "owner/repo?token=example", "owner/repo#x", (&url.URL{Scheme: "https", Host: "github.com", User: url.UserPassword("user", "example"), Path: "/owner/repo"}).String(), "http://github.com/owner/repo", "https://github.com/owner/repo/tree", "https://github.com/owner/repo/blob/main/x", "https://github.com/owner/repo/tree/main/%2fetc", "https://github.com/owner/repo/tree/../x", "https://github.com/owner/repo?", "https://github.com/owner/repo/", "https://github.com.example.com/owner/repo", "owner/repo/\x00", "owner/repo/"} {
		t.Run(ref, func(t *testing.T) {
			_, err := r.Resolve(t.Context(), dto.Request{Kind: "github", Ref: ref})
			requireProblem(t, err, "import.invalid-reference", 400)
		})
	}
	require.Zero(t, calls.Load())
}
func TestGitHubBoundedBodies(t *testing.T) {
	for _, mode := range []string{"declared", "chunked", "endless", "env"} {
		t.Run(mode, func(t *testing.T) {
			var written atomic.Int64
			r, _ := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if mode == "env" && !strings.HasSuffix(req.URL.Path, ".env.example") {
					_, _ = io.WriteString(w, validCompose)
					return
				}
				if mode == "declared" {
					w.Header().Set("Content-Length", fmt.Sprint(fileLimit+1))
				} else {
					w.(http.Flusher).Flush()
				}
				chunk := strings.Repeat("#", 8192)
				for i := 0; mode == "endless" || i < fileLimit/8192+1; i++ {
					n, err := io.WriteString(w, chunk)
					written.Add(int64(n))
					if err != nil || req.Context().Err() != nil {
						return
					}
					if mode != "declared" {
						w.(http.Flusher).Flush()
					}
				}
			}))
			_, err := importGitHub(r)
			requireProblem(t, err, "import.too-large", 413)
		})
	}
}
func TestGitHubSlowUpstream(t *testing.T) {
	for _, part := range []string{"headers", "body", "total"} {
		t.Run(part, func(t *testing.T) {
			release := make(chan struct{})
			r, _ := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if part == "body" {
					w.(http.Flusher).Flush()
				}
				timer := time.NewTimer(150 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-req.Context().Done():
				case <-release:
				case <-timer.C:
					_, _ = io.WriteString(w, validCompose)
				}
			}))
			defer close(release)
			if part == "total" {
				r.total = 30 * time.Millisecond
				r.perRequest = time.Second
			} else {
				r.total = time.Second
				r.perRequest = 30 * time.Millisecond
			}
			_, err := importGitHub(r)
			requireProblem(t, err, "import.timeout", 504)
		})
	}
}
func TestGitHubRedirectPolicy(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		foreignCalls.Add(1)
		_, _ = io.WriteString(w, validCompose)
	}))
	defer foreign.Close()
	for _, kind := range []string{"foreign", "scheme", "loop", "userinfo", "allowed"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			var origin string
			r, server := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				if kind == "allowed" && strings.HasSuffix(req.URL.Path, "/done") {
					_, _ = io.WriteString(w, validCompose)
					return
				}
				target := origin + "/loop"
				switch kind {
				case "foreign":
					target = foreign.URL + "/file"
				case "scheme":
					target = "https://" + strings.TrimPrefix(origin, "http://") + "/file"
				case "userinfo":
					target = strings.Replace(origin, "://", "://user:example@", 1) + "/file"
				case "allowed":
					target = origin + "/done"
				}
				http.Redirect(w, req, target, 302)
			}))
			origin = server.URL
			result, err := importGitHub(r)
			if kind == "allowed" {
				require.NoError(t, err)
				require.Equal(t, validCompose, result.ComposeContent)
			} else {
				requireProblem(t, err, "import.redirect-refused", 502)
			}
			if kind == "foreign" || kind == "scheme" || kind == "userinfo" {
				require.Equal(t, int32(1), calls.Load())
			}
			if kind == "loop" {
				require.Equal(t, int32(4), calls.Load())
			}
		})
	}
	require.Zero(t, foreignCalls.Load())
}
func TestGitHubRequestBudget(t *testing.T) {
	var calls atomic.Int32
	r, _ := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(req.URL.Path, "/hop") {
			w.WriteHeader(404)
			return
		}
		http.Redirect(w, req, "/hop", 302)
	}))
	_, err := importGitHub(r)
	requireProblem(t, err, "import.source-unavailable", 502)
	require.Equal(t, int32(6), calls.Load())
}
func TestGitHubNoProxyAndNeutralFailures(t *testing.T) {
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	r, err := NewResolver(nil, "")
	require.NoError(t, err)
	require.Nil(t, r.transport.(*http.Transport).Proxy)
	require.Zero(t, calls.Load())
	for status, code := range map[int]string{404: "import.not-found", 401: "import.source-unavailable", 500: "import.source-unavailable"} {
		r, _ := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "upstream secret diagnostic")
		}))
		_, err := importGitHub(r)
		requireProblem(t, err, code, map[bool]int{true: 404, false: 502}[status == 404])
	}
}

func TestImportBoundedReader(t *testing.T) {
	_, err := readFile(strings.NewReader(strings.Repeat("x", fileLimit+1)))
	requireProblem(t, err, "import.too-large", 413)
}

func TestImportTransportDiscardsErrorBodies(t *testing.T) {
	r, server := githubResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.com/token"`)
		w.WriteHeader(502)
		_, _ = io.WriteString(w, "upstream diagnostic must never reach library logging")
	}))
	budget := &budgetTransport{next: r.transport, perRequest: time.Second}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	res, err := (&http.Client{Transport: budget}).Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Empty(t, data, "neutralize upstream diagnostics before registry-library handling")
	require.Equal(t, 502, res.StatusCode)
	require.Equal(t, `Bearer realm="https://auth.example.com/token"`, res.Header.Get("WWW-Authenticate"), "preserve authentication negotiation")
}
