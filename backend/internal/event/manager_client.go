// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original tailnet manager dialer for the agent event HTTP fallback.

package event

import (
	"net/http"
	"time"
)

// managerHTTPClient uses the tailnet manager dialer when configured, without proxy env.
func (s *EventService) managerHTTPClient() *http.Client {
	if s.cfg == nil || s.cfg.ManagerDialer == nil {
		return s.httpClient
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{DialContext: s.cfg.ManagerDialer}}
}
