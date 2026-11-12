// Source: getarcaneapp/arcane/backend/internal/middleware/environment.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps, saved Git repositories and Swarm; validate original WebSocket client origin before forwarding credentials; enforce hub restore-point authority and lifecycle response contracts.; M12 backup upload requires both upload and backup permission before forwarding; M12 backup target set receiver is hub-only and never proxied; notification and Auto Heal contracts and delivery for #296; M14 container and host operation contracts; bounded typed agent refusal classification.; hub-owned one-shot restart wiring; M14 shell acceptance before remote upgrade; caller request restored after fenced forwarding; ordinary inventory reads time-bounded.; M15 resource authorization and isolated transport helpers.

package middleware

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	"github.com/labstack/echo/v5"
	"io"
	"log/slog"
	"net/http"
)

// createProxyRequest builds the HTTP request to forward to the remote environment.
func (m *EnvironmentMiddleware) createProxyRequest(c *echo.Context, target string, accessToken *string) (*http.Request, error) {
	srcReq := c.Request()
	validatedTarget, err := httpx.ValidateOutboundHTTPURL(target)
	if err != nil {
		return nil, common.Classify(common.ErrEnvironmentInvalidProxyTarget, fmt.Errorf("Invalid proxy target URL: %w", err)) //nolint:staticcheck // Preserve the existing error message.
	}

	// The body is streamed straight through rather than buffered: volume backup
	// and image import uploads run to gigabytes, and reading them into memory
	// cost roughly twice their size per in-flight request. This drops GetBody,
	// so the transport can no longer replay the body across a redirect or an
	// idle-connection retry; a proxied API call has no legitimate need for
	// either, and a failure surfaces as a 502 rather than silent corruption.
	//
	// A server request's ContentLength is 0 only when there is genuinely no
	// body (-1 means chunked/unknown), so it is the safe signal for whether to
	// forward one at all.
	var requestBody io.ReadCloser
	var contentLength int64
	switch {
	case srcReq.Body == nil:
	case srcReq.ContentLength != 0:
		requestBody, contentLength = srcReq.Body, srcReq.ContentLength
	default:
		_ = srcReq.Body.Close()
	}

	// The body is deliberately not logged: at debug level it would put compose
	// files and registry credentials into the ring buffer served by
	// /api/diagnostics/logs, and stringifying it allocated even when debug was off.
	slog.DebugContext(srcReq.Context(), "Creating proxy request", "method", srcReq.Method, "target", target, "contentLength", srcReq.ContentLength, "contentType", srcReq.Header.Get("Content-Type"))

	requestURL := *validatedTarget
	req := (&http.Request{
		Method:        srcReq.Method,
		URL:           &requestURL,
		Host:          requestURL.Host,
		Header:        make(http.Header),
		Body:          requestBody,
		ContentLength: contentLength,
	}).WithContext(srcReq.Context())

	skip := edge.GetSkipHeaders()
	edge.CopyRequestHeaders(srcReq.Header, req.Header, skip)
	remenv.StripHubCredentials(req.Header, m.proxyIdentityHeader)
	edge.SetAgentToken(req, accessToken)
	edge.SetForwardedHeaders(req, c.RealIP(), srcReq.Host)

	return req, nil
}

// writeProxyResponse copies the proxy response back to the client. An upstream
// 401 means the agent rejected the manager's credentials; passing it through
// makes browser clients treat it as a lost session, so it becomes a 502.
func (m *EnvironmentMiddleware) writeProxyResponse(c *echo.Context, resp *http.Response) error {
	if remenv.AgentAuthRejected(resp.StatusCode) {
		slog.WarnContext(c.Request().Context(), "Remote environment rejected the manager's credentials", "target", resp.Request.URL.Host)
		return c.JSON(http.StatusBadGateway, map[string]any{
			"success": false,
			"code":    remenv.ClassifyAgentRefusal(resp.StatusCode, remenv.ReadAgentRefusalBody(resp.Body)),
			"data":    map[string]any{"error": "Remote environment rejected the manager's credentials"},
		})
	}
	w := c.Response()
	hopByHop := edge.BuildHopByHopHeaders(resp.Header)
	edge.CopyResponseHeaders(resp.Header, w.Header(), hopByHop)

	w.WriteHeader(resp.StatusCode)
	if c.Request().Method != http.MethodHead {
		edge.CopyBodyWithFlush(w, resp.Body)
	}
	return nil
}
