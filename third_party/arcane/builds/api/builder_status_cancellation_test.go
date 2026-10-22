// Source: docklet-next/third_party/arcane/builds/api/builder_lifetime_test.go
// Revision: 351d9c3e4b79cf133d03b5d792723e1e87be3e63
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Real-client cancellation backpressure regression for issue #115.

package api

import (
	"context"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	controlapi "github.com/moby/buildkit/api/services/control"
	buildkit "github.com/moby/buildkit/client"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Only the renderer sees this context. Err observes its actual cancellation
// branch, rather than guessing that cancellation has been processed after a sleep.
type observedRendererContext struct {
	context.Context
	cancelled <-chan struct{}
	observed  chan struct{}
	once      sync.Once
}

func (c *observedRendererContext) Done() <-chan struct{} { return c.cancelled }

func (c *observedRendererContext) Err() error {
	select {
	case <-c.cancelled:
		c.once.Do(func() { close(c.observed) })
		return context.Canceled
	default:
		return nil
	}
}

type observedStatusStream struct {
	grpc.ClientStream
	received int
	overflow chan struct{}
	all      chan struct{}
}

func (s *observedStatusStream) RecvMsg(msg any) error {
	err := s.ClientStream.RecvMsg(msg)
	if err == nil {
		s.received++
		if s.received == 17 {
			close(s.overflow)
		}
		if s.received == 32 {
			close(s.all)
		}
	}
	return err
}

func TestStreamSolveStatusCancellationDrainsRealClient(t *testing.T) {
	overflow, all := make(chan struct{}), make(chan struct{})
	interceptor := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil || method != controlapi.Control_Status_FullMethodName {
			return stream, err
		}
		return &observedStatusStream{ClientStream: stream, overflow: overflow, all: all}, nil
	}
	control, provider, _, _, _ := newLifetimeBuild(t, nil, nil, buildkit.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptor)))
	burst, finish := make(chan struct{}), make(chan struct{})
	var burstOnce, finishOnce sync.Once
	control.statusTraffic = func(stream grpc.ServerStreamingServer[controlapi.StatusResponse]) error {
		select {
		case <-burst:
		case <-stream.Context().Done():
			return nil
		}
		for range 32 {
			if err := stream.Send(&controlapi.StatusResponse{}); err != nil {
				return err
			}
		}
		select {
		case <-finish:
		case <-stream.Context().Done():
		}
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	statuses := make(chan *buildkit.SolveStatus, 16)
	solveDone, rendererDone := make(chan struct{}), make(chan struct{})
	var solveErr, rendererErr error
	renderCtx := &observedRendererContext{Context: context.WithoutCancel(ctx), cancelled: ctx.Done(), observed: make(chan struct{})}
	go func() {
		defer close(rendererDone)
		rendererErr = streamSolveStatusInternal(renderCtx, statuses, io.Discard)
	}()
	go func() {
		defer close(solveDone)
		_, solveErr = provider.client.Solve(ctx, nil, buildkit.SolveOpt{Frontend: "dockerfile.v0"}, statuses)
	}()
	t.Cleanup(func() {
		cancel()
		burstOnce.Do(func() { close(burst) })
		finishOnce.Do(func() { close(finish) })
		// Recover the old blocked sender as well, so an assertion-red run joins
		// the real client instead of abandoning its goroutines.
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			for range statuses {
			}
		}()
		awaitLifetime(t, solveDone, "cleanup of real Solve client")
		awaitLifetime(t, rendererDone, "cleanup of real renderer")
		awaitLifetime(t, drained, "cleanup status drain")
	})
	awaitLifetime(t, control.solveEntered, "real Solve RPC entry")
	awaitLifetime(t, control.sessionEntered, "real Session RPC entry")
	cancel()
	awaitLifetime(t, renderCtx.observed, "plain renderer cancellation effect")
	burstOnce.Do(func() { close(burst) })
	select {
	case <-rendererDone:
		awaitLifetime(t, overflow, "seventeenth client-decoded status")
		// This diagnoses the actual client channel send, not server-side gRPC
		// buffering. The dependency is pinned, but no source line is assumed.
		var blocked string
		require.Eventually(t, func() bool {
			stack := make([]byte, 1<<20)
			n := runtime.Stack(stack, true)
			for _, goroutine := range strings.Split(string(stack[:n]), "\n\n") {
				if strings.Contains(goroutine, "[chan send]") && strings.Contains(goroutine, "github.com/moby/buildkit/client.(*Client).solve.func") && strings.Contains(goroutine, "/client/solve.go:") {
					blocked = goroutine
					return true
				}
			}
			return false
		}, lifetimeTestDeadline, time.Millisecond)
		t.Logf("client status buffer: %d/%d; blocked sender:\n%s", len(statuses), cap(statuses), blocked)
		t.Fatal("renderer returned before the real Solve client closed its status channel")
	case <-all:
		select {
		case <-rendererDone:
			t.Fatal("renderer must retain status ownership until real Solve completion")
		default:
		}
	case <-time.After(lifetimeTestDeadline):
		t.Fatal("real client did not receive the status burst")
	}
	finishOnce.Do(func() { close(finish) })
	awaitLifetime(t, control.statusDone, "real Status RPC completion")
	awaitLifetime(t, control.solveDone, "real Solve RPC completion")
	awaitLifetime(t, control.sessionDone, "real Session RPC completion")
	awaitLifetime(t, solveDone, "real Solve client completion")
	awaitLifetime(t, rendererDone, "renderer completion after status close")
	require.Equal(t, codes.Canceled, status.Code(solveErr))
	require.ErrorIs(t, rendererErr, context.Canceled)
	require.Empty(t, statuses, "all statuses must drain before the renderer joins")
}
