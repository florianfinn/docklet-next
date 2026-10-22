// Source: docklet-next/issues/112
// Revision: 729e98e0ebe9adff8c3f4a557238f27c849bf417
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original focused BuildKit RPC and status-renderer lifetime tests.

package api

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	controlapi "github.com/moby/buildkit/api/services/control"
	buildkit "github.com/moby/buildkit/client"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.getarcane.app/builds/types"
)

const lifetimeTestDeadline = 10 * time.Second

type lifetimeControlServer struct {
	controlapi.UnimplementedControlServer
	solveEntered chan struct{}
	solveRelease chan struct{}
	solveDone    chan struct{}
	statusSent   chan struct{}
	statusDone   chan struct{}
	sessionDone  chan struct{}
	solveErr     error
	requestMu    sync.Mutex
	request      *controlapi.SolveRequest
}

func (s *lifetimeControlServer) Solve(ctx context.Context, req *controlapi.SolveRequest) (*controlapi.SolveResponse, error) {
	s.requestMu.Lock()
	s.request = req
	s.requestMu.Unlock()
	close(s.solveEntered)
	defer close(s.solveDone)
	select {
	case <-s.solveRelease:
		if s.solveErr != nil {
			return nil, s.solveErr
		}
		return &controlapi.SolveResponse{ExporterResponse: map[string]string{"containerimage.digest": "sha256:synthetic"}}, nil
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, ctx.Err().Error())
	}
}

func (s *lifetimeControlServer) Status(_ *controlapi.StatusRequest, stream grpc.ServerStreamingServer[controlapi.StatusResponse]) error {
	defer close(s.statusDone)
	now := timestamppb.Now()
	err := stream.Send(&controlapi.StatusResponse{Vertexes: []*controlapi.Vertex{{
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Name:   "[internal] synthetic build step", Started: now, Completed: now,
	}}})
	if err != nil {
		return err
	}
	close(s.statusSent)
	select {
	case <-s.solveDone:
		return nil
	case <-stream.Context().Done():
		return nil
	}
}

func (s *lifetimeControlServer) Session(stream grpc.BidiStreamingServer[controlapi.BytesMessage, controlapi.BytesMessage]) error {
	defer close(s.sessionDone)
	for {
		if _, err := stream.Recv(); err != nil {
			return nil
		}
	}
}

func (s *lifetimeControlServer) solveRequest() *controlapi.SolveRequest {
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	return s.request
}

type lifetimeProvider struct {
	client *buildkit.Client
	closed chan error
	count  atomic.Int32
}

func (p *lifetimeProvider) Name() string { return "depot" }

func (p *lifetimeProvider) NewSession(context.Context, types.BuildRequest) (*buildSession, error) {
	return &buildSession{Client: p.client, Close: func(buildErr error) error {
		p.count.Add(1)
		p.closed <- buildErr
		return nil
	}}, nil
}

type lifetimeWriter struct {
	entered chan struct{}
	release chan struct{}
	failed  error
	once    sync.Once
	mu      sync.Mutex
	text    strings.Builder
	active  atomic.Bool
}

func (w *lifetimeWriter) Write(p []byte) (int, error) {
	w.active.Store(true)
	w.once.Do(func() { close(w.entered) })
	defer w.active.Store(false)
	<-w.release
	if w.failed != nil {
		return 0, w.failed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.Write(p)
}

func (w *lifetimeWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.String()
}

type lifetimeOutcome struct {
	result *types.BuildResult
	err    error
}

func awaitLifetime[T any](t *testing.T, ch <-chan T, label string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(lifetimeTestDeadline):
		t.Fatalf("timed out waiting for %s", label)
		var zero T
		return zero
	}
}

func newLifetimeBuild(t *testing.T, solveErr error, writerErr error) (*lifetimeControlServer, *lifetimeProvider, *lifetimeWriter, types.BuildRequest, *Service) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	control := &lifetimeControlServer{
		solveEntered: make(chan struct{}), solveRelease: make(chan struct{}), solveDone: make(chan struct{}),
		statusSent: make(chan struct{}), statusDone: make(chan struct{}),
		sessionDone: make(chan struct{}), solveErr: solveErr,
	}
	controlapi.RegisterControlServer(grpcServer, control)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() { grpcServer.Stop(); _ = listener.Close() })

	client, err := buildkit.New(t.Context(), "bufnet", buildkit.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	provider := &lifetimeProvider{client: client, closed: make(chan error, 1)}
	service := NewService(Config{SettingsProvider: testSettingsProviderInternal{}})
	service.providers["depot"] = provider
	writer := &lifetimeWriter{entered: make(chan struct{}), release: make(chan struct{}), failed: writerErr}
	t.Cleanup(func() {
		select {
		case <-writer.release:
		default:
			close(writer.release)
		}
		select {
		case <-control.solveRelease:
		default:
			close(control.solveRelease)
		}
	})

	contextDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644))
	req := types.BuildRequest{
		ContextDir: contextDir, Provider: "depot", Push: true,
		Tags: []string{"registry.example.com/app:test"},
	}
	return control, provider, writer, req, service
}

func TestBuildImageJoinsBuildkitStatusRenderer(t *testing.T) {
	cases := []struct {
		name      string
		solveErr  error
		writerErr error
		cancel    bool
	}{
		{name: "solve error", solveErr: status.Error(codes.InvalidArgument, `exporter "image" could not be found`)},
		{name: "cancellation", cancel: true},
		{name: "success"},
		{name: "writer error on success", writerErr: errors.New("synthetic writer failure")},
		{name: "writer error on solve failure", solveErr: status.Error(codes.InvalidArgument, `exporter "image" could not be found`), writerErr: errors.New("synthetic writer failure")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			control, provider, writer, req, service := newLifetimeBuild(t, tc.solveErr, tc.writerErr)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			outcomes := make(chan lifetimeOutcome, 1)
			go func() {
				result, err := service.BuildImage(ctx, req, writer, "")
				outcomes <- lifetimeOutcome{result, err}
			}()

			awaitLifetime(t, control.statusSent, "real Status RPC send")
			awaitLifetime(t, writer.entered, "actual progress writer entry")
			// Status delivery can precede Solve dispatch; cancellation must reach an entered RPC.
			awaitLifetime(t, control.solveEntered, "real Solve RPC entry")
			if tc.cancel {
				cancel()
			} else {
				close(control.solveRelease)
			}
			awaitLifetime(t, control.solveDone, "real Solve RPC completion")
			awaitLifetime(t, control.statusDone, "real Status RPC completion")
			awaitLifetime(t, control.sessionDone, "real Session RPC completion")
			// All RPCs have finished while the renderer remains inside Write.
			require.True(t, writer.active.Load())
			// The window is only an upper bound for detecting a premature return;
			// successful completion is checked after the writer is released.
			select {
			case <-provider.closed:
				t.Fatal("provider session closed while status writer was active")
			case <-outcomes:
				t.Fatal("BuildImage returned while status writer was active")
			case <-time.After(time.Second):
			}

			close(writer.release)
			outcome := awaitLifetime(t, outcomes, "BuildImage completion")
			closeErr := awaitLifetime(t, provider.closed, "provider session Close")
			require.False(t, writer.active.Load(), "renderer must finish before Close and return")
			require.EqualValues(t, 1, provider.count.Load())
			require.Equal(t, outcome.err, closeErr)
			require.NotNil(t, control.solveRequest(), "the real Solve RPC must execute")

			switch {
			case tc.cancel:
				require.Nil(t, outcome.result)
				require.Error(t, outcome.err)
				require.Equal(t, codes.Canceled, status.Code(outcome.err))
				require.Contains(t, outcome.err.Error(), context.Canceled.Error())
			case tc.solveErr != nil:
				require.Nil(t, outcome.result)
				var exporterErr *types.BuildKitImageExporterError
				require.ErrorAs(t, outcome.err, &exporterErr)
				require.Equal(t, "depot", exporterErr.ProviderName)
				require.Contains(t, outcome.err.Error(), `exporter "image" could not be found`)
				require.NotErrorIs(t, outcome.err, tc.writerErr)
			default:
				require.NoError(t, outcome.err)
				require.Equal(t, &types.BuildResult{Provider: "depot", Tags: req.Tags, Digest: "sha256:synthetic"}, outcome.result)
			}
			if tc.writerErr == nil && !tc.cancel {
				require.Contains(t, writer.String(), "synthetic build step")
			}
			request := control.solveRequest()
			require.Equal(t, "dockerfile.v0", request.Frontend)
			require.Equal(t, "image", request.Exporters[0].Type)
			require.Equal(t, "registry.example.com/app:test", request.Exporters[0].Attrs["name"])
		})
	}
}

var _ io.Writer = (*lifetimeWriter)(nil)
