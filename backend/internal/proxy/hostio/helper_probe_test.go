// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package hostio

import (
	"context"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type probeFixture struct {
	mu                         sync.Mutex
	fail, hold                 string
	version, body              string
	exit                       int
	creates, removes, versions int
	lists                      int
	image, network             string
	operation                  string
	afterLogs                  func()
	entered                    chan struct{}
	release                    chan struct{}
}

func helperFixture(t *testing.T) (*probeFixture, *ProbeRunner) {
	t.Helper()
	f := &probeFixture{version: `{"v":1,"version":"0.2.0","capabilities":["http","tls"]}`, body: helperHTTP(200), entered: make(chan struct{}, 1), release: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stage := ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/create"):
			stage = "create"
		case strings.HasSuffix(r.URL.Path, "/start"):
			stage = "start"
		case strings.HasSuffix(r.URL.Path, "/wait"):
			stage = "wait"
		case strings.HasSuffix(r.URL.Path, "/logs"):
			stage = "logs"
		case r.Method == "DELETE":
			stage = "remove"
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/containers/helper/json"):
			stage = "inspect"
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			stage = "list"
		}
		f.mu.Lock()
		fail, hold := f.fail, f.hold
		f.mu.Unlock()
		if stage == hold {
			select {
			case f.entered <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-f.release:
			}
			return
		}
		if stage == "start" && fail == "binary-missing" {
			http.Error(w, `{"message":"exec: /usr/bin/docklet-probe: stat /usr/bin/docklet-probe: no such file or directory"}`, 500)
			return
		}
		if stage == "start" && fail == "crun-missing" {
			http.Error(w, `{"message":"crun: synthetic wording for a command that was not found"}`, 400)
			return
		}
		if stage == "inspect" {
			code := 0
			if fail == "crun-missing" {
				code = 127
			}
			fmt.Fprintf(w, `{"Id":"helper","State":{"Status":"exited","ExitCode":%d}}`, code)
			return
		}
		if stage == "create" && fail == "image-pruned" {
			http.Error(w, `{"message":"No such image: sha256:local-image"}`, 404)
			return
		}
		if stage == fail {
			http.Error(w, "synthetic daemon error", 500)
			return
		}
		switch stage {
		case "create":
			var q struct {
				Image      string
				Cmd        []string
				Labels     map[string]string
				HostConfig struct {
					ReadonlyRootfs bool
					CapDrop        []string
					SecurityOpt    []string
					NetworkMode    string
					AutoRemove     bool
					Binds          []string
					Mounts         []any
				}
			}
			require.NoError(t, json.UnmarshalRead(r.Body, &q))
			require.True(t, q.HostConfig.ReadonlyRootfs)
			require.Equal(t, []string{"ALL"}, q.HostConfig.CapDrop)
			require.Equal(t, []string{"no-new-privileges:true"}, q.HostConfig.SecurityOpt)
			require.False(t, q.HostConfig.AutoRemove)
			require.Empty(t, q.HostConfig.Binds)
			require.Empty(t, q.HostConfig.Mounts)
			require.Equal(t, "probe", q.Labels[ProbeLabel])
			require.NotEmpty(t, q.Labels[ProbeOperationLabel])
			f.mu.Lock()
			f.creates++
			f.image = q.Image
			f.network = q.HostConfig.NetworkMode
			f.operation = q.Labels[ProbeOperationLabel]
			if len(q.Cmd) > 1 && q.Cmd[1] == "version" {
				f.versions++
			}
			f.mu.Unlock()
			if hold == "create-lost" {
				f.entered <- struct{}{}
				select {
				case <-r.Context().Done():
				case <-f.release:
				}
				return
			}
			if fail == "create-lost" {
				http.Error(w, "lost create response", 500)
				return
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"Id":"helper"}`)
		case "start":
			w.WriteHeader(204)
		case "wait":
			f.mu.Lock()
			exit := f.exit
			f.mu.Unlock()
			fmt.Fprintf(w, `{"StatusCode":%d}`, exit)
		case "logs":
			f.mu.Lock()
			body := f.body
			hook := f.afterLogs
			if f.network == "none" {
				body = f.version
			}
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			header := make([]byte, 8)
			header[0] = 1
			binary.BigEndian.PutUint32(header[4:], uint32(len(body)))
			if hold == "collect" {
				_, _ = w.Write(header)
				_, _ = w.Write([]byte(body[:1]))
				w.(http.Flusher).Flush()
				f.entered <- struct{}{}
				select {
				case <-r.Context().Done():
				case <-f.release:
				}
				return
			}
			_, _ = w.Write(header)
			_, _ = w.Write([]byte(body))
			if hook != nil {
				hook()
			}
		case "remove":
			f.mu.Lock()
			f.removes++
			f.mu.Unlock()
			w.WriteHeader(204)
		case "list":
			f.mu.Lock()
			operation := f.operation
			f.lists++
			f.mu.Unlock()
			fmt.Fprintf(w, `[{"Id":"orphan","Labels":{"docklet.helper":"probe","docklet.probe-operation":%q}},{"Id":"other","Labels":{}}]`, operation)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(f.release) })
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.54"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	return f, &ProbeRunner{Client: cli, Resolve: func(context.Context) (volumehelper.Image, error) {
		return volumehelper.Image{Ref: "registry.example.test/tools:0.2.0", ID: "sha256:local-image"}, nil
	}}
}
func helperHTTP(status int) string {
	return fmt.Sprintf(`{"v":1,"status":%d,"headers":{},"body":{"sha256":"%s","truncated":false}}`, status, strings.Repeat("a", 64))
}
func probeInstance() dto.Instance {
	return dto.Instance{ID: "instance", Identity: dto.Identity{ContainerID: "target", StartedAt: "2026-11-20T00:00:00Z"}}
}
func probeInspect(context.Context) (Epoch, error) {
	return Epoch{ContainerID: "target", StartedAt: "2026-11-20T00:00:00Z", Running: true}, nil
}
func noFallback(t *testing.T) func(context.Context) (Response, error) {
	return func(context.Context) (Response, error) { t.Error("unexpected wget fallback"); return Response{}, nil }
}

func TestP4CapabilityClassesAndImmutableCache(t *testing.T) {
	for _, tt := range []struct {
		name, version string
		exit          int
		want          dto.Reason
		bad           bool
	}{
		{name: "supported", version: `{"v":1,"version":"0.2.0","capabilities":["http","tls"]}`},
		{name: "missing", version: `{}`, exit: 127, want: dto.ProbeClientMissing},
		{name: "unsupported", version: `{"v":2,"version":"next","capabilities":["http","tls"]}`, want: dto.ProbeClientUnsupported},
		{name: "published-v1-missing-capabilities", version: `{"v":1,"version":"0.2.0"}`},
		{name: "explicit-no-http", version: `{"v":1,"version":"0.2.0","capabilities":["tls"]}`, want: dto.ProbeClientUnsupported},
		{name: "explicit-http-only", version: `{"v":1,"version":"0.2.0","capabilities":["http"]}`},
		{name: "malformed", version: `{`, want: dto.ProbeClientUnsupported},
		{name: "malformed-capabilities", version: `{"v":1,"version":"0.2.0","capabilities":null}`, want: dto.ProbeClientUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, r := helperFixture(t)
			f.version = tt.version
			f.exit = tt.exit
			p, err := r.Prepare(t.Context())
			if tt.bad {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, p.Reason)
			_, err = r.Prepare(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, f.versions)
			r.Resolve = func(context.Context) (volumehelper.Image, error) {
				return volumehelper.Image{Ref: p.Image.Ref, ID: "sha256:replacement"}, nil
			}
			_, err = r.Prepare(t.Context())
			require.NoError(t, err)
			require.Equal(t, 2, f.versions, "mutable reference must recheck a new local image ID")
		})
	}
	for _, absent := range []bool{false, true} {
		_, r := helperFixture(t)
		failure := errors.New("synthetic infrastructure failure")
		r.Resolve = func(context.Context) (volumehelper.Image, error) {
			if absent {
				return volumehelper.Image{}, &volumehelper.ImageUnavailable{Err: failure}
			}
			return volumehelper.Image{}, &volumehelper.DockerError{Err: failure}
		}
		p, err := r.Prepare(t.Context())
		if absent {
			require.NoError(t, err)
			require.Equal(t, dto.ProbeImageUnavailable, p.Reason)
		} else {
			require.ErrorIs(t, err, failure)
		}
	}
}

func TestP4MalformedProbeNeverFallsBack(t *testing.T) {
	f, r := helperFixture(t)
	p, err := r.Prepare(t.Context())
	require.NoError(t, err)
	f.body = `{"v":1,"status":200,"error":"timeout"}`
	_, err = r.Probe(t.Context(), p, probeInstance(), probeInspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, dto.NotObservable, problem.Code)
}
func TestP5LateSuccessDiscardedAfterEpochChange(t *testing.T) {
	f, r := helperFixture(t)
	p, err := r.Prepare(t.Context())
	require.NoError(t, err)
	// Collection completes before this fence reads the changed epoch.
	reads := 0
	inspect := func(ctx context.Context) (Epoch, error) {
		epoch, _ := probeInspect(ctx)
		reads++
		if reads > 1 {
			epoch.StartedAt = "2026-11-20T00:01:00Z"
		}
		return epoch, nil
	}
	_, err = r.Probe(t.Context(), p, probeInstance(), inspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, dto.ContainerChanged, problem.Code)
	require.Equal(t, f.creates, f.removes)
}
func TestP6EveryHelperFailureCleansUp(t *testing.T) {
	for _, version := range []bool{false, true} {
		for _, stage := range []string{"create", "create-lost", "start", "wait", "logs", "collect"} {
			t.Run(fmt.Sprintf("version=%t/%s", version, stage), func(t *testing.T) {
				f, r := helperFixture(t)
				if stage == "collect" {
					f.body = strings.Repeat("x", MaxProbeJSON+1)
					f.version = f.body
				} else {
					f.fail = stage
				}
				if version {
					_, _ = r.Prepare(t.Context())
				} else {
					_, _ = r.Probe(t.Context(), PreparedProbe{Image: volumehelper.Image{ID: "sha256:local-image"}}, probeInstance(), probeInspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				require.Equal(t, f.creates, f.removes, "every created helper must be removed")
			})
		}
	}
}
func TestP6P11CancellationCleansUpDetached(t *testing.T) {
	for _, version := range []bool{false, true} {
		for _, stage := range []string{"create", "create-lost", "start", "wait", "logs", "collect"} {
			t.Run(fmt.Sprintf("version=%t/%s", version, stage), func(t *testing.T) {
				f, r := helperFixture(t)
				f.hold = stage
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					if version {
						_, err := r.Prepare(ctx)
						done <- err
					} else {
						_, err := r.Probe(ctx, PreparedProbe{Image: volumehelper.Image{ID: "sha256:local-image"}}, probeInstance(), probeInspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
						done <- err
					}
				}()
				select {
				case <-f.entered:
				case <-time.After(3 * time.Second):
					t.Fatal("helper stage did not start")
				}
				cancel()
				select {
				case err := <-done:
					require.Error(t, err)
				case <-time.After(3 * time.Second):
					t.Fatal("helper did not honor cancellation")
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				require.Equal(t, f.creates, f.removes, "cancelled request must use detached removal context")
				if stage == "create" {
					require.Positive(t, f.lists, "uncertain creation must sweep its operation label")
				}
			})
		}
	}
}
func TestP8AgentLocalImageAndSweep(t *testing.T) {
	f, r := helperFixture(t)
	p, err := r.Prepare(t.Context())
	require.NoError(t, err)
	response, err := r.Probe(t.Context(), p, probeInstance(), probeInspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
	require.NoError(t, err)
	require.Equal(t, "sha256:local-image", f.image)
	require.Equal(t, "container:target", f.network)
	require.Equal(t, f.image, response.Probe.ImageID)
	before := f.removes
	require.NoError(t, r.Sweep(t.Context()))
	require.Equal(t, before+1, f.removes)
}

type probeTransport struct {
	hook func(*http.Request)
	base http.RoundTripper
}

func (p probeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.hook(r)
	return p.base.RoundTrip(r)
}
func TestP11HelperAndVersionUseOwnDeadlines(t *testing.T) {
	f, r := helperFixture(t)
	budget := ProbeVersionBudget
	cli, err := client.New(client.WithHost(r.Client.DaemonHost()), client.WithAPIVersion("1.54"), client.WithHTTPClient(&http.Client{Transport: probeTransport{base: http.DefaultTransport, hook: func(q *http.Request) {
		if !strings.HasSuffix(q.URL.Path, "/start") {
			return
		}
		deadline, ok := q.Context().Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), budget)
	}}}))
	require.NoError(t, err)
	defer cli.Close()
	r.Client = cli
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	prepared, err := r.Prepare(ctx)
	require.NoError(t, err)
	budget = HelperProbeBudget
	_, err = r.Probe(ctx, prepared, probeInstance(), probeInspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
	require.NoError(t, err)
	require.Equal(t, f.creates, f.removes)
}
func TestP4ExplicitHTTPOnlyFallsBackForTLS(t *testing.T) {
	f, r := helperFixture(t)
	f.version = `{"v":1,"version":"0.2.0","capabilities":["http"]}`
	f.body = strings.ReplaceAll(tlsHTTP(), "tls.example.test", "example.test")
	p, err := r.Prepare(t.Context())
	require.NoError(t, err)
	require.Empty(t, p.Reason)
	require.False(t, p.SupportsTLS)
	called := false
	response, err := r.Probe(t.Context(), p, probeInstance(), probeInspect, "example.test", "https://127.0.0.1/", "", func(context.Context) (Response, error) { called = true; return Response{Status: 200}, nil })
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, dto.ProbeClientUnsupported, response.Probe.Reason)
}

func TestP4MissingBinaryAtContainerStartFallsBack(t *testing.T) {
	f, r := helperFixture(t)
	f.fail = "binary-missing"
	prepared, err := r.Prepare(t.Context())
	require.NoError(t, err)
	require.Equal(t, dto.ProbeClientMissing, prepared.Reason)
	require.Equal(t, f.creates, f.removes)
}

func TestP6CleanupFailureRemainsTransientAndStartupSweepRecovers(t *testing.T) {
	f, r := helperFixture(t)
	f.fail = "remove"
	_, err := r.Prepare(t.Context())
	var failure *volumehelper.DockerError
	require.ErrorAs(t, err, &failure)
	f.fail = ""
	require.NoError(t, r.Sweep(t.Context()))
	require.Equal(t, f.creates, f.removes)
}

// N5: a missing probe binary is detected from the runtime's exit code, so the
// wget fallback also works with crun wording.
func TestP4MissingBinaryDetectedByExitCode(t *testing.T) {
	f, r := helperFixture(t)
	f.fail = "crun-missing"
	prepared, err := r.Prepare(t.Context())
	require.NoError(t, err)
	require.Equal(t, dto.ProbeClientMissing, prepared.Reason)
	require.Equal(t, f.creates, f.removes)
}

// N1: a definitive daemon refusal (pruned image) is a typed observation
// failure, never an endless transient retry.
func TestP6DaemonRefusalIsTyped(t *testing.T) {
	f, r := helperFixture(t)
	p, err := r.Prepare(t.Context())
	require.NoError(t, err)
	f.mu.Lock()
	f.fail = "image-pruned"
	f.mu.Unlock()
	_, err = r.Probe(t.Context(), p, probeInstance(), probeInspect, "example.test", "http://127.0.0.1/", "", noFallback(t))
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, dto.NotObservable, problem.Code)
	require.False(t, ProbeTransient(err))
}

// N2: hosts the helper cannot express (M27 accepted them through wget) fall
// back as an unsupported client instead of invalid-input.
func TestP4HelperArgumentRefusalFallsBack(t *testing.T) {
	for _, tt := range []struct{ host, url string }{
		{"my_app.example.test", "http://127.0.0.1/"},
		{"192.0.2.10", "https://127.0.0.1/"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			f, r := helperFixture(t)
			p, err := r.Prepare(t.Context())
			require.NoError(t, err)
			created := f.creates
			used := false
			response, err := r.Probe(t.Context(), p, probeInstance(), probeInspect, tt.host, tt.url, "", func(context.Context) (Response, error) {
				used = true
				return Response{}, nil
			})
			require.NoError(t, err)
			require.True(t, used)
			require.Equal(t, dto.ProbeClientUnsupported, response.Probe.Reason)
			require.Equal(t, created, f.creates, "no helper runs for refused arguments")
		})
	}
}
