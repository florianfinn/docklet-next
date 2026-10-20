// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original mapping of bounded producer records to existing WS JSON.

package ws

import (
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"go.getarcane.app/docker"
	"strings"
	"time"
)

type sourceLogLine struct {
	record     docker.LogRecord
	structured bool
	failure    bool
}

func normalizeSourceLogLine(line sourceLogLine, legacy func(string) wshub.LogMessage) wshub.LogMessage {
	var msg wshub.LogMessage
	if line.structured {
		// Only a leading valid Docker timestamp is metadata. Application text and
		// whitespace after it remain verbatim; stream names are never severity.
		msg = legacy("")
		msg.Message = line.record.Message
		if head, rest, ok := strings.Cut(msg.Message, " "); ok {
			if ts, err := time.Parse(time.RFC3339Nano, head); err == nil {
				msg.Timestamp = ts.UTC().Format(time.RFC3339Nano)
				msg.Message = rest
			}
		}
		msg.Service = line.record.Service
		msg.Stream = line.record.Stream
	} else {
		msg = legacy(line.record.Message)
		msg.Stream = "unknown"
	}
	switch msg.Level {
	case "error", "warn", "debug", "info":
	default:
		msg.Level = "info"
	}
	if line.failure {
		msg.Level = "error"
	}
	msg.Truncated = line.record.Truncated
	return msg
}
