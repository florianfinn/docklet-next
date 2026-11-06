// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const fileLimit = 256 << 10
const totalBudget = 10 * time.Second
const requestBudget = 5 * time.Second
const requestLimit = 6

type budgetTransport struct {
	next       http.RoundTripper
	count      atomic.Int32
	perRequest time.Duration
}

func (b *budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if b.count.Add(1) > requestLimit {
		return nil, dto.Failure("import.source-unavailable", 502)
	}
	ctx, cancel := context.WithTimeout(req.Context(), b.perRequest)
	res, err := b.next.RoundTrip(req.Clone(ctx))
	if err != nil {
		cancel()
		return nil, importFailure(err)
	}
	// Registry libraries may log error diagnostics during authentication or retries.
	// Keep status and challenge headers, but never hand them an upstream error body.
	if res.StatusCode >= 400 {
		_ = res.Body.Close()
		res.Body = http.NoBody
		res.ContentLength = 0
	}
	res.Body = &cancelBody{ReadCloser: res.Body, cancel: cancel}
	return res, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { defer b.cancel(); return b.ReadCloser.Close() }

func readFile(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, fileLimit+1))
	if len(data) > fileLimit {
		return nil, dto.Failure("import.too-large", 413)
	}
	if err != nil {
		return nil, importFailure(err)
	}
	return data, nil
}
func importFailure(err error) error {
	if p, ok := errors.AsType[*dto.Problem](err); ok {
		return p
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return dto.Failure("import.timeout", 504)
	}
	return dto.Failure("import.source-unavailable", 502)
}
