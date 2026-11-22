// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage.

package image

import (
	"bytes"
	"io"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

// composePullReader observes bounded daemon JSON lines before the renderer reads
// them. It returns exactly the source bytes and errors to preserve raw output.
type composePullReader struct {
	source    io.Reader
	reporter  *projects.DeployReporter
	image     string
	pending   []byte
	oversized bool
}

func (r *composePullReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	remaining := p[:n]
	for len(remaining) > 0 {
		index := bytes.IndexByte(remaining, '\n')
		part := remaining
		if index >= 0 {
			part = remaining[:index]
		}
		if !r.oversized {
			if len(r.pending)+len(part) > 64*1024 {
				r.pending = nil
				r.oversized = true
			} else {
				r.pending = append(r.pending, part...)
			}
		}
		if index < 0 {
			break
		}
		if !r.oversized {
			r.reporter.ObservePull(r.image, r.pending)
		}
		r.pending, r.oversized = r.pending[:0], false
		remaining = remaining[index+1:]
	}
	if err == io.EOF && len(r.pending) > 0 && !r.oversized {
		r.reporter.ObservePull(r.image, r.pending)
		r.pending = nil
	}
	return n, err
}
