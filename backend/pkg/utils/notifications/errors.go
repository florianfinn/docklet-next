// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notifications

import (
	"context"
	"errors"
	"net"
	"net/textproto"
	"strings"
)

func ClassifyDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "delivery-unreachable"
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return "delivery-unreachable"
	}
	if e, ok := errors.AsType[*textproto.Error](err); ok {
		if e.Code == 530 || e.Code == 535 {
			return "delivery-auth"
		}
		return "delivery-rejected"
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"401", "403", "authentication", "unauthorized", "invalid token", "invalid_auth"} {
		if strings.Contains(msg, s) {
			return "delivery-auth"
		}
	}
	for _, s := range []string{"status", "rejected", "smtp reply"} {
		if strings.Contains(msg, s) {
			return "delivery-rejected"
		}
	}
	for _, s := range []string{"timeout", "no such host", "connection refused", "tls", "connect:"} {
		if strings.Contains(msg, s) {
			return "delivery-unreachable"
		}
	}
	return "delivery-invalid"
}
