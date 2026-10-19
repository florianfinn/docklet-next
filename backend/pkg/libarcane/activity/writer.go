// Source: getarcaneapp/arcane/backend/pkg/libarcane/activity/writer.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; raw line capture, bounded blocking queue and trailing line flush; additive runtime seal, error and drain join.

package activity

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/samber/mo"
	kit "go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type Writer struct {
	ctx context.Context
	// persistCtx is ctx detached from cancellation. Cancellation usually
	// races the last — most diagnostic — output lines (the error frames
	// explaining it), so batches persist on persistCtx while ctx only bounds
	// the drain goroutine's intake, mirroring CompleteActivity's detached
	// terminal write.
	persistCtx      context.Context
	activityService MessageAppender
	activityID      string
	writer          io.Writer
	defaultStep     string
	queueCh         chan writerQueueItem

	runtime *runtimeWriterInternal

	mu     sync.Mutex
	buffer []byte
}

const (
	writerAppendQueueSize = 128
	// writerAppendBatchSize caps how many queued lines are drained into one
	// AppendMessages transaction.
	writerAppendBatchSize = 32
)

type writerAppendMessage struct {
	level   activitytypes.MessageLevel
	message string
	payload database.JSON
	step    string
	output  *activitytypes.OutputLine
}

type writerQueueItem struct {
	message *writerAppendMessage
	flush   chan struct{}
	finish  bool
}

func NewWriter(ctx context.Context, activityService MessageAppender, activityID string, writer io.Writer, defaultStep string) io.Writer {
	if activityService == nil || strings.TrimSpace(activityID) == "" {
		return kit.Ternary(writer == nil, io.Discard, writer)
	}
	if existing, ok := writer.(*Writer); ok {
		if existing.runtime == nil || existing.activityID == strings.TrimSpace(activityID) {
			return existing
		}
		// A different run must never write into its predecessor's capture.
		writer = io.Discard
	}
	persistCtx := ctx
	if persistCtx != nil {
		persistCtx = context.WithoutCancel(persistCtx)
	}
	out := &Writer{
		ctx:             ctx,
		persistCtx:      persistCtx,
		activityService: activityService,
		activityID:      strings.TrimSpace(activityID),
		writer:          writer,
		defaultStep:     strings.TrimSpace(defaultStep),
		queueCh:         make(chan writerQueueItem, writerAppendQueueSize),
	}
	go out.drainMessagesInternal(ctx)
	return out
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.runtime != nil {
		return w.writeRuntimeInternal(p)
	}
	if w.writer != nil {
		// Keep activity capture alive when the client-side response stream disconnects.
		_, _ = w.writer.Write(p)
	}

	w.mu.Lock()
	var messages []writerAppendMessage
	w.buffer = append(w.buffer, p...)
	for {
		idx := bytes.IndexByte(w.buffer, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buffer[:idx])
		w.buffer = w.buffer[idx+1:]
		messages = append(messages, w.captureLineInternal(line))
	}
	w.mu.Unlock()

	for _, message := range messages {
		w.enqueueMessageInternal(message)
	}

	return len(p), nil
}

func (w *Writer) Flush() {
	if w.runtime != nil {
		w.flushRuntimeInternal()
		return
	}
	if flusher, ok := w.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	w.mu.Lock()
	var tail *writerAppendMessage
	if len(w.buffer) > 0 {
		item := w.captureLineInternal(string(w.buffer))
		tail = &item
		w.buffer = nil
	}
	w.mu.Unlock()
	if tail != nil {
		w.enqueueMessageInternal(*tail)
	}
	flushDone := make(chan struct{})
	select {
	case w.queueCh <- writerQueueItem{flush: flushDone}:
	case <-doneInternal(w.ctx):
		return
	}
	select {
	case <-flushDone:
	case <-doneInternal(w.ctx):
		return
	}
}

func (w *Writer) captureLineInternal(line string) writerAppendMessage {
	message, ok := w.processLineInternal(strings.TrimSpace(line)).Get()
	output := &activitytypes.OutputLine{
		Timestamp: time.Now().UTC(),
		Stream:    "stdout",
		Text:      line,
		Step:      w.defaultStep,
	}
	if ok {
		output.Level = message.level
		if message.level == activitytypes.MessageLevelError {
			output.Stream = "stderr"
		}
	}
	message.output = output
	return message
}

// processLineInternal normalizes stream lines into coded activity messages.
// Raw lines are captured separately before this conversion.
func (w *Writer) processLineInternal(line string) mo.Option[writerAppendMessage] {
	if line == "" || w.activityService == nil || w.activityID == "" {
		return mo.None[writerAppendMessage]()
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return mo.Some(writerAppendMessage{
			level:   activitytypes.MessageLevelInfo,
			message: line,
			step:    w.defaultStep,
		})
	}

	if errorValue, ok := payload["error"]; ok && errorValue != nil {
		return mo.Some(writerAppendMessage{
			level:   activitytypes.MessageLevelError,
			message: kit.ToString(errorValue),
			payload: payload,
			step:    w.defaultStep,
		})
	}

	if logValue, ok := payload["log"]; ok {
		message := kit.ToString(logValue)
		if strings.TrimSpace(message) == "" {
			return mo.None[writerAppendMessage]()
		}
		return mo.Some(writerAppendMessage{
			level:   activitytypes.MessageLevelInfo,
			message: message,
			step:    w.defaultStep,
		})
	}

	return mo.Some(writerAppendMessage{
		level:   activitytypes.MessageLevelInfo,
		message: line,
		payload: payload,
		step:    w.defaultStep,
	})
}

func (w *Writer) enqueueMessageInternal(message writerAppendMessage) {
	// Queue backpressure preserves accepted output; cancellation ends a blocked write.
	select {
	case w.queueCh <- writerQueueItem{message: &message}:
	case <-doneInternal(w.ctx):
		return
	}
}

func (w *Writer) drainMessagesInternal(ctx context.Context) {
	for {
		select {
		case item := <-w.queueCh:
			w.drainQueueBatchInternal(item)
		case <-doneInternal(ctx):
			// Cancellation stops intake, but the lines already accepted into
			// the queue — typically the ones explaining the cancellation —
			// still get persisted before the goroutine exits.
			w.drainRemainingInternal()
			return
		}
	}
}

func (w *Writer) drainRemainingInternal() {
	for {
		select {
		case item := <-w.queueCh:
			w.drainQueueBatchInternal(item)
		default:
			return
		}
	}
}

// drainQueueBatchInternal writes queued raw lines and messages in one batch.
// Flush signals encountered while draining close only after the write, since
// the messages queued before them are only then persisted.
func (w *Writer) drainQueueBatchInternal(item writerQueueItem) {
	var batch []AppendMessageRequest
	var flushes []chan struct{}

	collect := func(queued writerQueueItem) {
		if queued.flush != nil {
			flushes = append(flushes, queued.flush)
			return
		}
		if queued.message == nil {
			return
		}
		batch = append(batch, AppendMessageRequest{
			Level:   queued.message.level,
			Message: queued.message.message,
			Payload: queued.message.payload,
			Step:    queued.message.step,
			Output:  queued.message.output,
		})
	}

	collect(item)
drain:
	for len(batch) < writerAppendBatchSize {
		select {
		case queued := <-w.queueCh:
			collect(queued)
		default:
			break drain
		}
	}

	if len(batch) > 0 && w.persistCtx != nil {
		if _, err := w.activityService.AppendMessages(w.persistCtx, w.activityID, batch); err != nil {
			slog.WarnContext(w.persistCtx, "failed to persist activity output batch", "activityId", w.activityID, "lines", len(batch), "error", err)
		}
	}
	// Flushes release even when the append failed: Flush carries no error
	// path, and holding them would stall activity completion on the very
	// cancellation that typically caused the failure.
	for _, flushDone := range flushes {
		close(flushDone)
	}
}

func doneInternal(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

// Runtime capture owns its output independently from execution cancellation.
// ingress serializes extraction through enqueue; the drain never acquires it.
type runtimeWriterInternal struct {
	ingress    sync.Mutex
	finishOnce sync.Once
	done       chan struct{}
	sealed     bool // protected by Writer.mu, as is firstError
	firstError error
}

var ErrWriterFinished = errors.New("runtime output is finished")
var ErrWriterNotRuntime = errors.New("writer is not a runtime capture")

const runtimeAppendTimeout = 10 * time.Second

// NewRuntimeWriter captures only worker-owned output; no response/socket writer
// is called. MessageAppender stays unchanged. Each actual append has its own
// bounded context, preserving values while detaching execution cancellation.
func NewRuntimeWriter(ctx context.Context, service MessageAppender, activityID, defaultStep string) (*Writer, error) {
	if ctx == nil || service == nil || strings.TrimSpace(activityID) == "" {
		return nil, errors.New("runtime capture requires context, appender and run")
	}
	w := &Writer{ctx: ctx, persistCtx: context.WithoutCancel(ctx), activityService: service,
		activityID: strings.TrimSpace(activityID), writer: io.Discard, defaultStep: strings.TrimSpace(defaultStep),
		queueCh: make(chan writerQueueItem, writerAppendQueueSize), runtime: &runtimeWriterInternal{done: make(chan struct{})}}
	go w.drainRuntimeInternal()
	return w, nil
}

// Done closes only after every accepted item and the final tail have drained.
// Legacy writers have no completion contract and return a nil channel.
func (w *Writer) Done() <-chan struct{} {
	if w.runtime == nil {
		return nil
	}
	return w.runtime.done
}

// Finish seals intake once. A timed-out caller does not terminate the finisher
// or the drain: callers must retain ownership until Done actually closes.
func (w *Writer) Finish(waitCtx context.Context) error {
	if w.runtime == nil {
		return ErrWriterNotRuntime
	}
	w.runtime.finishOnce.Do(func() {
		w.mu.Lock()
		w.runtime.sealed = true
		w.mu.Unlock()
		go func() {
			w.runtime.ingress.Lock()
			w.enqueueRuntimeTailInternal()
			w.queueCh <- writerQueueItem{finish: true}
			w.runtime.ingress.Unlock()
		}()
	})
	select {
	case <-w.runtime.done:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.runtime.firstError
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

func (w *Writer) writeRuntimeInternal(p []byte) (int, error) {
	w.runtime.ingress.Lock()
	defer w.runtime.ingress.Unlock()
	w.mu.Lock()
	if w.runtime.sealed {
		w.mu.Unlock()
		return 0, ErrWriterFinished
	}
	var messages []writerAppendMessage
	w.buffer = append(w.buffer, p...)
	for {
		idx := bytes.IndexByte(w.buffer, '\n')
		if idx < 0 {
			break
		}
		messages = append(messages, w.captureLineInternal(string(w.buffer[:idx])))
		w.buffer = w.buffer[idx+1:]
	}
	w.mu.Unlock()
	for i := range messages {
		w.queueCh <- writerQueueItem{message: &messages[i]}
	}
	return len(p), nil
}

func (w *Writer) enqueueRuntimeTailInternal() {
	w.mu.Lock()
	var tail *writerAppendMessage
	if len(w.buffer) > 0 {
		item := w.captureLineInternal(string(w.buffer))
		tail = &item
		w.buffer = nil
	}
	w.mu.Unlock()
	if tail != nil {
		w.queueCh <- writerQueueItem{message: tail}
	}
}

func (w *Writer) flushRuntimeInternal() {
	w.runtime.ingress.Lock()
	w.mu.Lock()
	sealed := w.runtime.sealed
	w.mu.Unlock()
	if sealed {
		w.runtime.ingress.Unlock()
		<-w.runtime.done
		return
	}
	w.enqueueRuntimeTailInternal()
	flushed := make(chan struct{})
	w.queueCh <- writerQueueItem{flush: flushed}
	w.runtime.ingress.Unlock()
	<-flushed
}

func (w *Writer) drainRuntimeInternal() {
	defer close(w.runtime.done)
	for {
		item := <-w.queueCh
		var batch []AppendMessageRequest
		var flushes []chan struct{}
		finished := false
		collect := func(item writerQueueItem) {
			finished = finished || item.finish
			if item.flush != nil {
				flushes = append(flushes, item.flush)
			}
			if m := item.message; m != nil {
				batch = append(batch, AppendMessageRequest{Level: m.level, Message: m.message, Payload: m.payload, Step: m.step, Output: m.output})
			}
		}
		collect(item)
	collectBatch:
		for !finished && len(batch) < writerAppendBatchSize {
			select {
			case next := <-w.queueCh:
				collect(next)
			default:
				break collectBatch
			}
		}
		if len(batch) > 0 {
			ctx, cancel := context.WithTimeout(w.persistCtx, runtimeAppendTimeout)
			_, err := w.activityService.AppendMessages(ctx, w.activityID, batch)
			err = errors.Join(err, ctx.Err())
			cancel()
			if err != nil {
				w.mu.Lock()
				if w.runtime.firstError == nil {
					w.runtime.firstError = err
				}
				w.mu.Unlock()
			}
		}
		for _, flushed := range flushes {
			close(flushed)
		}
		if finished {
			return
		}
	}
}
