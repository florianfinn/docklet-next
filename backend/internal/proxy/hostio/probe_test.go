// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original probe parser tests on outputs measured in baseline B0 (BusyBox v1.37.0).

package hostio

import (
	"errors"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

const (
	measuredOK       = "Connecting to 127.0.0.1 (127.0.0.1:443)\n  HTTP/1.1 200 OK\n  Content-Length: 354\n  Content-Type: text/plain; charset=utf-8\n  Date: Sun, 15 Nov 2026 00:35:34 GMT\n  Connection: close\n  \nwriting to stdout\n-                    100% |********************************|   354  0:00:00 ETA\nwritten to stdout\n"
	measuredNotFound = "Connecting to 127.0.0.1 (127.0.0.1:80)\n  HTTP/1.1 404 Not Found\nwget: server returned error: HTTP/1.1 404 Not Found\n"
	measuredRedirect = "Connecting to 127.0.0.1 (127.0.0.1:80)\n  HTTP/1.1 301 Moved Permanently\n  Location: https://method.example.com/\nwget: bad address 'method.example.com'\n"
	measuredOption   = "wget: unrecognized option: max-redirect=0\nBusyBox v1.37.0 multi-call binary.\n"
)

func failed(stderr string) (ExecResult, error) {
	return ExecResult{Stderr: []byte(stderr), ExitCode: 1}, dto.Fail(dto.ExecFailed, 422)
}

func TestParseProbeMeasuredOutputs(t *testing.T) {
	ok, err := ParseProbe(ExecResult{Stdout: []byte("example body"), Stderr: []byte(measuredOK)}, nil)
	require.NoError(t, err)
	require.Equal(t, 200, ok.Status)
	require.Equal(t, map[string]string{"content-length": "354", "content-type": "text/plain; charset=utf-8"}, ok.Headers)
	require.NotEmpty(t, ok.BodyHash)
	require.False(t, ok.Followed)

	notFound, err := ParseProbe(failed(measuredNotFound))
	require.NoError(t, err, "an HTTP error status is an answer, not a failed probe")
	require.Equal(t, 404, notFound.Status)
	require.Empty(t, notFound.BodyHash)

	redirect, err := ParseProbe(failed(measuredRedirect))
	require.NoError(t, err)
	require.Equal(t, 301, redirect.Status)
	require.Equal(t, "https://method.example.com/", redirect.Headers["location"])
}

// Only the first response counts; the body of a followed redirect is never evidence.
func TestParseProbeKeepsOnlyTheFirstResponse(t *testing.T) {
	stderr := "Connecting to 127.0.0.1 (127.0.0.1:80)\n  HTTP/1.1 302 Found\n  Location: http://127.0.0.1/next\n  \nConnecting to 127.0.0.1 (127.0.0.1:80)\n  HTTP/1.1 200 OK\n  Content-Type: text/html\n  \nwriting to stdout\n"
	got, err := ParseProbe(ExecResult{Stdout: []byte("example later body"), Stderr: []byte(stderr)}, nil)
	require.NoError(t, err)
	require.Equal(t, 302, got.Status)
	require.Equal(t, map[string]string{"location": "http://127.0.0.1/next"}, got.Headers)
	require.True(t, got.Followed)
	require.Empty(t, got.BodyHash)
}

func TestParseProbeUnavailable(t *testing.T) {
	_, err := ParseProbe(failed(measuredOption))
	requireReason(t, err, dto.ProbeOptionUnavailable)
	_, err = ParseProbe(ExecResult{}, errors.New(`exec: "wget": executable file not found in $PATH`))
	requireReason(t, err, dto.ProbeUnavailable)
	_, err = ParseProbe(ExecResult{Stderr: []byte("sh: wget: not found\n"), ExitCode: 127}, dto.Fail(dto.ExecFailed, 422))
	requireReason(t, err, dto.ProbeUnavailable)
	_, err = ParseProbe(ExecResult{}, dto.Fail(dto.ExecRunning, 409))
	requireReason(t, err, dto.ExecRunning)
	got, err := ParseProbe(failed("Connecting to 127.0.0.1 (127.0.0.1:80)\nwget: can't connect to remote host (127.0.0.1): Connection refused\n"))
	require.NoError(t, err)
	require.Zero(t, got.Status, "no listener is no HTTP response")
	_, err = ParseProbe(failed("  HTTP/1.1 abc\n"))
	requireReason(t, err, dto.NotObservable)
}

func TestDiffersAndGatewayErrors(t *testing.T) {
	base := Response{Status: 200, Headers: map[string]string{"content-type": "text/plain"}, BodyHash: "a"}
	require.False(t, Differs(base, base))
	require.True(t, Differs(base, Response{Status: 404}))
	require.True(t, Differs(base, Response{Status: 200, Headers: map[string]string{"content-type": "text/html"}, BodyHash: "a"}))
	require.True(t, Differs(base, Response{Status: 200, Headers: map[string]string{"content-type": "text/plain", "server": "example"}, BodyHash: "a"}))
	require.True(t, Differs(base, Response{Status: 200, Headers: map[string]string{"content-type": "text/plain"}, BodyHash: "b"}))
	require.False(t, Differs(base, Response{Status: 200, Headers: map[string]string{"content-type": "text/plain"}}), "a missing body hash is no difference")
	for _, s := range []int{502, 503, 504} {
		require.True(t, GatewayError(s))
	}
	require.False(t, GatewayError(500))
}

func requireReason(t *testing.T, err error, code dto.Reason) {
	t.Helper()
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, code, p.Code)
}
