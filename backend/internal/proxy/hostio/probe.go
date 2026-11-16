// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original shared probe evidence for the Traefik and Caddy engines (BusyBox wget -S output, baseline B0).

package hostio

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// Response is the first response of a probe. BusyBox wget follows redirects and
// cannot be told not to, so only the first status line and its headers count;
// the body hash is kept only when no further response was fetched.
type Response struct {
	// Status 0 means no HTTP response at all (connection refused or reset).
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	BodyHash string            `json:"bodyHash,omitempty"`
	Followed bool              `json:"followed,omitempty"`
}

// probeHeaders are compared between probes; volatile headers such as Date are not.
var probeHeaders = map[string]bool{"location": true, "content-type": true, "content-length": true, "server": true}

// ParseProbe reads the result of Exec(…, "probe", …). A non-zero exit with a status
// line is an ordinary HTTP answer (BusyBox exits 1 on 4xx and 5xx).
func ParseProbe(res ExecResult, err error) (Response, error) {
	text := string(res.Stderr)
	if err != nil {
		var p *dto.Problem
		if !errors.As(err, &p) || p.Code != dto.ExecFailed {
			if strings.Contains(err.Error(), "executable file not found") {
				return Response{}, dto.Fail(dto.ProbeUnavailable, 409)
			}
			return Response{}, err
		}
	}
	if strings.Contains(text, "unrecognized option") || strings.Contains(text, "invalid option") {
		return Response{}, dto.Fail(dto.ProbeOptionUnavailable, 409)
	}
	var out Response
	responses := 0
	inHeaders := false
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "  ") {
			inHeaders = false
			continue
		}
		field := strings.TrimSpace(line)
		if strings.HasPrefix(field, "HTTP/") {
			responses++
			inHeaders = responses == 1
			if inHeaders {
				parts := strings.Fields(field)
				if len(parts) < 2 {
					return Response{}, dto.Fail(dto.NotObservable, 409)
				}
				code, convErr := strconv.Atoi(parts[1])
				if convErr != nil || code < 100 || code > 599 {
					return Response{}, dto.Fail(dto.NotObservable, 409)
				}
				out.Status = code
			}
			continue
		}
		if field == "" {
			inHeaders = false
			continue
		}
		if inHeaders {
			name, value, ok := strings.Cut(field, ":")
			if name = strings.ToLower(strings.TrimSpace(name)); ok && probeHeaders[name] {
				if out.Headers == nil {
					out.Headers = map[string]string{}
				}
				out.Headers[name] = strings.TrimSpace(value)
			}
		}
	}
	if responses == 0 {
		if res.ExitCode == 127 || strings.Contains(text, "wget: not found") {
			return Response{}, dto.Fail(dto.ProbeUnavailable, 409)
		}
		return Response{}, nil
	}
	out.Followed = responses > 1
	if !out.Followed && len(res.Stdout) > 0 {
		h := sha256.Sum256(res.Stdout)
		out.BodyHash = hex.EncodeToString(h[:])
	}
	return out, nil
}

// GatewayError is a first response that never counts as evidence of a working change.
func GatewayError(status int) bool { return status == 502 || status == 503 || status == 504 }

// Differs reports whether the first response after a change differs from the one
// before it: status, compared headers, or the body hash when both have one.
func Differs(before, after Response) bool {
	if before.Status != after.Status || len(before.Headers) != len(after.Headers) {
		return true
	}
	for k, v := range before.Headers {
		if after.Headers[k] != v {
			return true
		}
	}
	return before.BodyHash != "" && after.BodyHash != "" && before.BodyHash != after.BodyHash
}
