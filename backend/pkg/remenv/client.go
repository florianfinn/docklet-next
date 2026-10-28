// Source: getarcaneapp/arcane/backend/pkg/remenv/client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package remenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/types/vulnerability"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderAPIKey        = "X-Api-Key"            // #nosec G101: header name, not a credential
	HeaderAgentToken    = "X-Arcane-Agent-Token" // #nosec G101: header name, not a credential
	HeaderAuthorization = "Authorization"
	bearerScheme        = "Bearer "
)

var ErrEnvironmentUnavailable = errors.New("edge agent is not connected")

// ExtractBearerToken returns the token portion of an "Authorization: Bearer <token>"
// header value. It is case-insensitive on the scheme and trims surrounding whitespace.
// Returns an empty string if the header is empty or does not use the Bearer scheme.
func ExtractBearerToken(headerValue string) string {
	headerValue = strings.TrimSpace(headerValue)
	if headerValue == "" {
		return ""
	}
	if len(headerValue) < len(bearerScheme) || !strings.EqualFold(headerValue[:len(bearerScheme)], bearerScheme) {
		return ""
	}
	return strings.TrimSpace(headerValue[len(bearerScheme):])
}

// RedactedTokenFingerprint returns a short, redacted identifier for a token
// suitable for debug logs. It is intentionally not reversible and never
// reveals the full secret.
func RedactedTokenFingerprint(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 10 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-4:]
}

type Request struct {
	ResponseLimit int64
	EnvironmentID string
	IsEdge        bool
	Method        string
	URL           string
	Path          string
	Headers       map[string]string
	Body          []byte
}

type Response struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

type TunnelTransport interface {
	EnsureAvailable(ctx context.Context, envID string) error
	Do(ctx context.Context, envID, method, path string, headers map[string]string, body []byte) (*Response, error)
}

type TunnelTransportFuncs struct {
	EnsureAvailableFunc func(ctx context.Context, envID string) error
	DoFunc              func(ctx context.Context, envID, method, path string, headers map[string]string, body []byte) (*Response, error)
}

func (t TunnelTransportFuncs) EnsureAvailable(ctx context.Context, envID string) error {
	if t.EnsureAvailableFunc == nil {
		return errors.New("edge transport unavailable")
	}
	return t.EnsureAvailableFunc(ctx, envID)
}

func (t TunnelTransportFuncs) Do(ctx context.Context, envID, method, path string, headers map[string]string, body []byte) (*Response, error) {
	if t.DoFunc == nil {
		return nil, errors.New("edge transport unavailable")
	}
	return t.DoFunc(ctx, envID, method, path, headers, body)
}

type Client struct {
	httpClient *http.Client
	tunnel     TunnelTransport
}

func NewClient(httpClient *http.Client, tunnel TunnelTransport) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		httpClient: httpClient,
		tunnel:     tunnel,
	}
}

func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if req.IsEdge {
		return c.doViaTunnelInternal(ctx, req)
	}

	return c.doDirectHTTPInternal(ctx, req)
}

func (c *Client) doViaTunnelInternal(ctx context.Context, req Request) (*Response, error) {
	if c.tunnel == nil {
		return nil, &TransportError{Err: errors.New("edge transport unavailable")}
	}

	if err := c.tunnel.EnsureAvailable(ctx, req.EnvironmentID); err != nil {
		return nil, &TransportError{Err: err}
	}

	resp, err := c.tunnel.Do(ctx, req.EnvironmentID, req.Method, req.Path, cloneHeaders(req.Headers), req.Body)
	if err != nil {
		return nil, &TransportError{Err: err}
	}

	return resp, nil
}

func (c *Client) doDirectHTTPInternal(ctx context.Context, req Request) (*Response, error) {
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, &TransportError{Err: fmt.Errorf("failed to create request: %w", err)}
	}

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	var reader io.Reader = resp.Body
	if req.ResponseLimit > 0 {
		reader = io.LimitReader(reader, req.ResponseLimit+1)
	}
	respBody, err := io.ReadAll(reader)
	if req.ResponseLimit > 0 && int64(len(respBody)) > req.ResponseLimit {
		return nil, vulnerability.Fail("vulnerability.response-too-large", 502)
	}
	if err != nil {
		return nil, &TransportError{Err: fmt.Errorf("failed to read response body: %w", err)}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    flattenHeaders(resp.Header),
	}, nil
}

func (r *Response) RequireSuccess() error {
	if r != nil && r.StatusCode >= http.StatusOK && r.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	if r == nil {
		return &StatusError{}
	}

	return &StatusError{
		StatusCode: r.StatusCode,
		Body:       r.Body,
	}
}

type TransportError struct {
	Err error
}

// Unavailable reports connection failures that permit using locally stored data.
// Invalid requests and failures reading an established response are not outages.
func (e *TransportError) Unavailable() bool {
	if e == nil || errors.Is(e.Err, context.Canceled) {
		return false
	}
	if errors.Is(e.Err, context.DeadlineExceeded) || errors.Is(e.Err, ErrEnvironmentUnavailable) {
		return true
	}
	var networkErr net.Error
	if errors.As(e.Err, &networkErr) && networkErr.Timeout() {
		return true
	}
	var operationErr *net.OpError
	if !errors.As(e.Err, &operationErr) || operationErr.Op != "dial" {
		return false
	}
	var addressErr *net.AddrError
	return !errors.As(operationErr, &addressErr)
}

func (e *TransportError) Error() string {
	if e == nil || e.Err == nil {
		return "transport error"
	}
	return e.Err.Error()
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type StatusError struct {
	StatusCode int
	Body       []byte
}

func (e *StatusError) Error() string {
	if e == nil {
		return "unexpected status code"
	}
	if body := strings.TrimSpace(string(e.Body)); body != "" {
		return fmt.Sprintf("unexpected status code %d: %s", e.StatusCode, body)
	}
	return fmt.Sprintf("unexpected status code %d", e.StatusCode)
}

type DecodeError struct {
	Err error
}

func (e *DecodeError) Error() string {
	if e == nil || e.Err == nil {
		return "failed to decode response body"
	}
	return fmt.Sprintf("failed to decode response body: %v", e.Err)
}

func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ApplyAgentTokenHeaders installs the target agent's X-header credentials.
// Hub forwarding strips user credentials first. Outbound agent registration
// sets its own Bearer token separately when a reverse proxy needs it.
func ApplyAgentTokenHeaders(headers http.Header, accessToken *string) {
	if headers == nil || accessToken == nil || strings.TrimSpace(*accessToken) == "" {
		return
	}

	headers.Set(HeaderAgentToken, *accessToken)
	headers.Set(HeaderAPIKey, *accessToken)
}

// ApplyAgentTokenHeaderMap installs the same credentials in map-based carriers.
func ApplyAgentTokenHeaderMap(headers map[string]string, accessToken *string) {
	if headers == nil || accessToken == nil || strings.TrimSpace(*accessToken) == "" {
		return
	}

	headers[HeaderAgentToken] = *accessToken
	headers[HeaderAPIKey] = *accessToken
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(headers))
	maps.Copy(out, headers)
	return out
}
