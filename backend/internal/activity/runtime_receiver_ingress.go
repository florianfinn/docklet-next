// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"context"
	"net/http"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

const RuntimeAdmissionLimit = 5 * time.Minute

type runtimeIngressKey struct{}

// RuntimeReceiverIngress starts before authentication, decoding or handler waits.
// Edge adds its own earlier ingress context before scheduling/body assembly.
func RuntimeReceiverIngress(next http.Handler, budget time.Duration) http.Handler {
	if budget <= 0 || budget > RuntimeAdmissionLimit {
		budget = RuntimeAdmissionLimit
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), budget)
		defer cancel()
		deadline, _ := ctx.Deadline()
		controller := http.NewResponseController(w)
		bounded := edge.RuntimeIngressBodyBounded(req.Context())
		if !bounded && controller.SetReadDeadline(deadline) != nil {
			runtimeWriteAPIError(w, http.StatusServiceUnavailable, "Runtime ingress unavailable")
			return
		}
		if !bounded {
			defer controller.SetReadDeadline(time.Time{})
		}
		joined := make(chan struct{})
		stop := context.AfterFunc(ctx, func() {
			defer close(joined)
			if !bounded {
				_ = controller.SetReadDeadline(time.Now())
			}
			if req.Body != nil {
				_ = req.Body.Close()
			}
		})
		defer func() {
			if !stop() {
				<-joined
			}
		}()
		ctx = context.WithValue(ctx, runtimeIngressKey{}, true)
		req = req.WithContext(ctx)
		req.Body = http.MaxBytesReader(w, req.Body, activitytypes.RuntimeMaxBodyBytes)
		next.ServeHTTP(w, req)
	})
}
