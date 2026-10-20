// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

const runtimeCaptureLineLimit = 256 << 10
const runtimeCaptureCut = " [runtime output truncated]"

func (w *Writer) runtimeCaptureLineInternal() writerAppendMessage {
	text := strings.ToValidUTF8(string(w.buffer), "\uFFFD")
	for len(text) > runtimeCaptureLineLimit-len(runtimeCaptureCut) {
		_, n := utf8.DecodeLastRuneInString(text)
		text = text[:len(text)-n]
		w.runtime.lineTruncated = true
	}
	if w.runtime.lineTruncated {
		text += runtimeCaptureCut
	}
	item := w.captureLineInternal(text)
	item.outputTruncated = w.runtime.lineTruncated
	w.runtime.lineTruncated = false
	return item
}
func (w *Writer) writeRuntimeInternal(p []byte) (int, error) {
	w.runtime.ingress.Lock()
	defer w.runtime.ingress.Unlock()
	w.mu.Lock()
	sealed := w.runtime.sealed
	w.mu.Unlock()
	if sealed {
		return 0, ErrWriterFinished
	}
	original := len(p)
	for len(p) > 0 {
		w.mu.Lock()
		messages := make([]writerAppendMessage, 0, writerAppendBatchSize)
		for len(p) > 0 && len(messages) < writerAppendBatchSize {
			newline := bytes.IndexByte(p, '\n')
			part := p
			if newline >= 0 {
				part = p[:newline]
			}
			remaining := runtimeCaptureLineLimit - len(runtimeCaptureCut) - len(w.buffer)
			if len(part) > remaining {
				w.buffer = append(w.buffer, part[:remaining]...)
				w.runtime.lineTruncated = true
			} else {
				w.buffer = append(w.buffer, part...)
			}
			if newline < 0 {
				p = nil
				break
			}
			messages = append(messages, w.runtimeCaptureLineInternal())
			w.buffer = w.buffer[:0]
			p = p[newline+1:]
		}
		w.mu.Unlock()
		for i := range messages {
			w.queueCh <- writerQueueItem{message: &messages[i]}
		}
	}
	return original, nil
}
