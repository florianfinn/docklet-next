// Source: docklet-next/issues/330
// Revision: a37d77d05c92bde3a4eab0cf6fa47ab36411e011
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original M16 rule for agent requests that fail because the agent is stopping.

package bootstrap

import (
	"context"
	"net/http"
	"sync/atomic"
)

// ShutdownSignal is set before the application stops anything, so request failures that the stop causes can be told
// apart from failures of a running agent.
type ShutdownSignal struct{ stopping atomic.Bool }

func newShutdownSignalInternal() *ShutdownSignal { return &ShutdownSignal{} }

// Begin marks that the application is about to stop.
func (s *ShutdownSignal) Begin() { s.stopping.Store(true) }

func (s *ShutdownSignal) begun() bool { return s.stopping.Load() }

// appLifetimeInternal ends the application context when the parent ends, on fatal failure or when the application
// stops. The shutdown signal is always set first: request and session work derives from this context, and its
// cancellation must already count as caused by the stop.
type appLifetimeInternal struct {
	ctx     context.Context
	end     context.CancelCauseFunc
	release func() bool
}

func newAppLifetimeInternal(parent context.Context, signal *ShutdownSignal) appLifetimeInternal {
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(parent))
	end := func(cause error) {
		signal.Begin()
		cancel(cause)
	}
	release := context.AfterFunc(parent, func() { end(context.Cause(parent)) })
	return appLifetimeInternal{ctx: ctx, end: end, release: release}
}

// agentShutdownInternal withholds a server failure that an agent request has not yet committed once shutdown has begun.
// The request ends like a lost connection (http.ErrAbortHandler; the Edge tunnel sends nothing), which every hub
// consumer reports as the host being unavailable. Committed responses, successes and client errors pass through.
func agentShutdownInternal(signal *ShutdownSignal, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &shutdownResponseInternal{ResponseWriter: w, signal: signal}
		next.ServeHTTP(writer, r)
		if writer.withheld {
			panic(http.ErrAbortHandler)
		}
	})
}

type shutdownResponseInternal struct {
	http.ResponseWriter
	signal    *ShutdownSignal
	committed bool
	withheld  bool
}

func (w *shutdownResponseInternal) WriteHeader(code int) {
	if w.withheld {
		return
	}
	if !w.committed && code >= http.StatusInternalServerError && w.signal.begun() {
		w.withheld = true
		return
	}
	if !w.committed {
		w.committed = code >= http.StatusOK || code == http.StatusSwitchingProtocols
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *shutdownResponseInternal) Write(p []byte) (int, error) {
	if w.withheld {
		return len(p), nil
	}
	w.committed = true
	return w.ResponseWriter.Write(p)
}

func (w *shutdownResponseInternal) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok && !w.withheld {
		w.committed = true
		f.Flush()
	}
}

func (w *shutdownResponseInternal) Unwrap() http.ResponseWriter { return w.ResponseWriter }
