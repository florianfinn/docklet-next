// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original backward-compatible close metadata in the existing tunnel error field.

package edge

import (
	"encoding/json/v2"
	"errors"
	"github.com/coder/websocket"
)

// Both JSON and protobuf already carry StreamClose.Error. Encoding optional
// close metadata there preserves the v11 envelope and older plain-text errors.
type streamClose struct {
	Code   websocket.StatusCode `json:"code"`
	Reason string               `json:"reason"`
}

func encodeStreamClose(err error) string {
	var closeError websocket.CloseError
	status, reason := websocket.StatusNormalClosure, ""
	if errors.As(err, &closeError) {
		status, reason = closeError.Code, closeError.Reason
	}
	if status == websocket.StatusNoStatusRcvd {
		status = websocket.StatusNormalClosure
	}
	if status < 1000 || status > 4999 || status == 1004 || status == 1006 || status == 1015 {
		status = websocket.StatusPolicyViolation
	}
	data, _ := json.Marshal(streamClose{status, reason})
	return string(data)
}
func decodeStreamClose(data string) (websocket.StatusCode, string) {
	var close streamClose
	if json.Unmarshal([]byte(data), &close) == nil && (close.Code >= 1000 && close.Code <= 4999 && close.Code != 1004 && close.Code != 1005 && close.Code != 1006 && close.Code != 1015) {
		return close.Code, close.Reason
	}
	if data == "" {
		return websocket.StatusNormalClosure, ""
	}
	if len(data) > 123 {
		data = data[:123]
	}
	return websocket.StatusInternalError, data
}
