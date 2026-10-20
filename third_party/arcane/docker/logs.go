// Source: getarcaneapp/kit/docker/logs.go
// Revision: e3d12f637964a6edd495f9b31a10d2f0a40fa127
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Bounded UTF-8 line/frame reading and discard through newline;
// follow and snapshot share incremental demultiplexing; optional source records.

package docker

import (
	"context"
	"io"
)

// StreamContainerLogs retains TTY/stream metadata before legacy text formatting.
func StreamContainerLogs(ctx context.Context, logs io.ReadCloser, logsChan chan<- string, follow, isTTY bool) error {
	return streamLogRecords(ctx, logs, logsChan, isTTY, "")
}

// StreamMultiplexedLogs merges completed stdout/stderr lines in arrival order.
func StreamMultiplexedLogs(ctx context.Context, logs io.Reader, logsChan chan<- string) error {
	return streamLogRecords(ctx, logs, logsChan, false, "")
}

// ReadAllLogs drains incrementally; no complete stdout/stderr output is retained.
func ReadAllLogs(ctx context.Context, logs io.ReadCloser, logsChan chan<- string) error {
	return streamLogRecords(ctx, logs, logsChan, false, "")
}

// ReadLogLines retains a bounded prefix and discards excess through the newline.
func ReadLogLines(ctx context.Context, reader io.Reader, logsChan chan<- string, prefix string) error {
	return streamLogRecords(ctx, reader, logsChan, true, prefix)
}

func streamLogRecords(ctx context.Context, reader io.Reader, lines chan<- string, tty bool, prefix string) error {
	// Joining the cancellation watcher prevents it from closing a reused reader
	// after this source has returned. A blocking reader must implement io.Closer.
	done := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			if closer, ok := reader.(io.Closer); ok {
				_ = closer.Close()
			}
		case <-done:
		}
	}()
	defer func() { close(done); <-joined }()
	sink := LogSinkFromContext(ctx)
	boundedPrefix, prefixCut := BoundLogText(prefix, MaxLogLineBytes-len(LogTruncatedMarker))
	return readLogRecords(ctx, reader, tty, func(record LogRecord) error {
		if sink != nil {
			return sink(record)
		}
		text := LogText(record)
		if prefix != "" {
			bounded, cut := BoundLogText(boundedPrefix+text, MaxLogLineBytes-len(LogTruncatedMarker))
			text = bounded
			if cut || prefixCut {
				text += LogTruncatedMarker
			}
		}
		select {
		case lines <- text:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
