// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original manager dialer injection for agent-to-hub transports (tailnet route).

package edge

import "net/http"

// applyManagerDialerInternal routes a manager transport through the configured dialer without proxies.
func applyManagerDialerInternal(cfg *Config, transport *http.Transport) *http.Transport {
	if cfg != nil && cfg.ManagerDialer != nil {
		transport.Proxy = nil
		transport.DialContext = cfg.ManagerDialer
	}
	return transport
}
