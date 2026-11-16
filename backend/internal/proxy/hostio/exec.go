// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package hostio

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	ExecTimeout  = 30 * time.Second
	ProbeTimeout = 10 * time.Second
)

func SafeValue(v string) bool {
	if v == "" || strings.HasPrefix(v, "-") {
		return false
	}
	for _, c := range v {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

// Args builds the whole vector; callers cannot supply flags or a shell fragment.
func Args(kind dto.Kind, action, file, host, rawURL string) ([]string, time.Duration, error) {
	fail := func() ([]string, time.Duration, error) { return nil, 0, dto.Fail(dto.InvalidInput, 422) }
	switch action {
	case "version":
		if kind == dto.Caddy || kind == dto.Traefik {
			return []string{string(kind), "version"}, ExecTimeout, nil
		}
	case "validate", "adapt", "reload":
		if kind != dto.Caddy || !SafeValue(file) || !strings.HasPrefix(file, "/") {
			return fail()
		}
		a := []string{"caddy", action, "--config", file, "--adapter", "caddyfile"}
		if action == "adapt" {
			a = append(a, "--pretty")
		}
		return a, ExecTimeout, nil
	case "probe", "admin":
		if !SafeValue(rawURL) || !SafeValue(host) {
			return fail()
		}
		u, err := url.Parse(rawURL)
		if err != nil || u.User != nil || u.Fragment != "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() != "127.0.0.1" {
			return fail()
		}
		// A Host header is a single field value, never a second option or header.
		if strings.ContainsAny(host, " /\\") {
			return fail()
		}
		// -Y off: a proxy variable in the container environment never redirects the probe.
		a := []string{"wget", "-Y", "off", "-S", "-T", "10", "-O", "-", "--header", "Host: " + host}
		if u.Scheme == "https" {
			a = append(a, "--no-check-certificate")
		}
		a = append(a, u.String())
		return a, ProbeTimeout, nil
	}
	return fail()
}

type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}
type Executor func(context.Context, []string, int) (ExecResult, error)

func Exec(ctx context.Context, run Executor, kind dto.Kind, action, file, host, rawURL string) (ExecResult, error) {
	args, budget, err := Args(kind, action, file, host, rawURL)
	if err != nil {
		return ExecResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	limit := MaxExecBytes
	if action == "probe" {
		limit = 64 << 10
	}
	res, err := run(ctx, args, limit)
	if err != nil {
		if ctx.Err() != nil {
			return res, dto.Fail(dto.ExecRunning, 409)
		}
		return res, err
	}
	if len(res.Stdout)+len(res.Stderr) > limit {
		return ExecResult{}, dto.Fail(dto.InputLimit, 422)
	}
	if res.ExitCode != 0 {
		return res, dto.Fail(dto.ExecFailed, 422)
	}
	return res, nil
}

// LimitOutput fails on overflow rather than accepting truncated validation text.
func LimitOutput(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxExecBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxExecBytes {
		return nil, dto.Fail(dto.InputLimit, 422)
	}
	return b, nil
}
