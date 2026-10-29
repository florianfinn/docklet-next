// Source: getarcaneapp/arcane/backend/pkg/utils/notifications/utils.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package notifications

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nicholas-fedor/shoutrrr"
	shoutrrrTypes "github.com/nicholas-fedor/shoutrrr/pkg/types"
)

// SanitizeForEmail sanitizes text for safe use in email subjects
func SanitizeForEmail(text string) string {
	// Remove control characters and newlines
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	// Trim whitespace
	return strings.TrimSpace(text)
}

func sendShoutrrrInternal(provider, destinationURL, message string, params *shoutrrrTypes.Params) error {
	sender, err := shoutrrr.CreateSenderWithOptions(shoutrrrTypes.SenderOptions{HTTPClient: notificationHTTPClient}, destinationURL)
	if err != nil {
		return fmt.Errorf("failed to create shoutrrr %s sender: %w", provider, err)
	}
	return waitDelivery(context.Background(), DeliveryTimeout, func() error {
		for _, err := range sender.Send(message, params) {
			if err != nil {
				return fmt.Errorf("failed to send %s message via shoutrrr: %w", provider, err)
			}
		}
		return nil
	})
}

const DeliveryTimeout = 15 * time.Second

var notificationHTTPClient = &http.Client{Timeout: DeliveryTimeout}

// waitDelivery bounds caller ownership for SDKs without context-aware Send.
func waitDelivery(ctx context.Context, timeout time.Duration, send func() error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- send() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func providerHost(host string, port int) string {
	if port > 0 {
		return net.JoinHostPort(host, strconv.Itoa(port))
	}
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}
