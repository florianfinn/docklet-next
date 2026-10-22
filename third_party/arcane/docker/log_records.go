// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original bounded Docker log records and optional in-process sink.

package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const MaxLogLineBytes = 64 * 1024
const LogTruncatedMarker = " [log line truncated]"
const daemonDiagnosticTruncatedMarker = " [daemon diagnostic truncated]"
const stderrPrefix = "[STDERR] "
const logContentLimit = MaxLogLineBytes - len(LogTruncatedMarker) - len(stderrPrefix)

// LogRecord carries metadata known at the producer, before text formatting.
type LogRecord struct {
	Message   string `json:"message"`
	Stream    string `json:"stream"`
	Service   string `json:"service,omitempty"`
	Truncated bool   `json:"truncated,omitzero"`
}
type logSinkKey struct{}
type LogSink func(LogRecord) error

// WithLogSink opts a log source into records while retaining its legacy text API.
func WithLogSink(ctx context.Context, sink LogSink) context.Context {
	return context.WithValue(ctx, logSinkKey{}, sink)
}
func LogSinkFromContext(ctx context.Context) LogSink {
	sink, _ := ctx.Value(logSinkKey{}).(LogSink)
	return sink
}

// BoundLogText only allocates in proportion to the retained prefix. Invalid or
// partial UTF-8 is replaced before the final byte limit is applied.
func BoundLogText(text string, limit int) (string, bool) {
	cut := len(text) > limit
	if cut {
		text = text[:limit]
	}
	text = strings.ToValidUTF8(text, "\uFFFD")
	if len(text) > limit {
		text = text[:limit]
		cut = true
	}
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text, cut
}
func LogText(record LogRecord) string {
	prefix := ""
	if record.Stream == "stderr" {
		prefix = stderrPrefix
	}
	text, cut := BoundLogText(record.Message, MaxLogLineBytes-len(prefix)-len(LogTruncatedMarker))
	if record.Truncated || cut {
		text += LogTruncatedMarker
	}
	return prefix + text
}

type pendingLogLine struct {
	bytes []byte
	cut   bool
}

func (p *pendingLogLine) add(chunk []byte) {
	n := min(len(chunk), logContentLimit-len(p.bytes))
	p.bytes = append(p.bytes, chunk[:n]...)
	p.cut = p.cut || n < len(chunk)
}
func (p *pendingLogLine) take(stream string) LogRecord {
	text, cut := BoundLogText(strings.TrimRight(string(p.bytes), "\r\n"), logContentLimit)
	record := LogRecord{Message: text, Stream: stream, Truncated: p.cut || cut}
	p.bytes = p.bytes[:0]
	p.cut = false
	return record
}

// LogReader consumes fixed-size chunks, never a frame-sized allocation. Each
// stream retains one bounded prefix and discards excess until its newline.
type LogReader struct {
	reader    *bufio.Reader
	tty       bool
	remaining uint32
	stream    int
	pending   [2]pendingLogLine
	terminal  error
	drain     int
}

func NewLogReader(reader io.Reader, tty bool) *LogReader {
	return &LogReader{reader: bufio.NewReaderSize(reader, 4096), tty: tty}
}
func (r *LogReader) Next(ctx context.Context) (LogRecord, error) {
	for {
		if err := ctx.Err(); err != nil {
			return LogRecord{}, err
		}
		if r.terminal != nil {
			for r.drain < len(r.pending) {
				i := r.drain
				r.drain++
				if len(r.pending[i].bytes) > 0 || r.pending[i].cut {
					return r.pending[i].take(r.name(i)), nil
				}
			}
			return LogRecord{}, r.terminal
		}
		if !r.tty && r.remaining == 0 {
			var header [8]byte
			if _, err := io.ReadFull(r.reader, header[:]); err != nil {
				r.terminal = err
				continue
			}
			if header[0] == 3 {
				return LogRecord{}, r.daemonError(ctx, binary.BigEndian.Uint32(header[4:]))
			}
			if header[0] != 1 && header[0] != 2 {
				return LogRecord{}, fmt.Errorf("invalid Docker log stream %d", header[0])
			}
			r.stream = int(header[0]) - 1
			r.remaining = binary.BigEndian.Uint32(header[4:])
			continue
		}
		// Peek is bounded by the reader capacity, even for a multi-gigabyte frame.
		n := 4096
		if !r.tty {
			n = min(n, int(r.remaining))
		}
		_, err := r.reader.Peek(1)
		if err != nil {
			if errors.Is(err, io.EOF) && !r.tty && r.remaining > 0 {
				err = io.ErrUnexpectedEOF
			}
			r.terminal = err
			continue
		}
		n = min(n, r.reader.Buffered())
		chunk, _ := r.reader.Peek(n)
		end := bytes.IndexByte(chunk, '\n')
		complete := end >= 0
		if complete {
			n = end + 1
			chunk = chunk[:n]
		}
		r.pending[r.stream].add(chunk)
		_, _ = r.reader.Discard(n)
		if !r.tty {
			r.remaining -= uint32(n)
		}
		if complete {
			record := r.pending[r.stream].take(r.name(r.stream))
			if record.Message != "" || record.Truncated {
				return record, nil
			}
		}
	}
}

// System-error frames are terminal diagnostics, never application log records.
// Read only a fixed prefix, even when the daemon advertises a huge payload.
func (r *LogReader) daemonError(ctx context.Context, size uint32) error {
	const limit = 1024
	prefix := make([]byte, min(size, limit))
	n, err := io.ReadFull(r.reader, prefix)
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	text, cut := BoundLogText(string(prefix[:n]), limit-len(daemonDiagnosticTruncatedMarker))
	if cut || size > limit {
		text += daemonDiagnosticTruncatedMarker
	}
	diagnostic := "Docker daemon log error: " + text
	if err != nil {
		return fmt.Errorf("%s: %w", diagnostic, err)
	}
	return errors.New(diagnostic)
}

func (r *LogReader) name(stream int) string {
	if r.tty {
		return "combined"
	}
	if stream == 1 {
		return "stderr"
	}
	return "stdout"
}

func readLogRecords(ctx context.Context, reader io.Reader, tty bool, emit LogSink) error {
	records := NewLogReader(reader, tty)
	for {
		record, err := records.Next(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = emit(record); err != nil {
			return err
		}
	}
}
