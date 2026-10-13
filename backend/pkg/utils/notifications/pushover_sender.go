// Source: getarcaneapp/arcane/backend/pkg/utils/notifications/pushover_sender.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package notifications

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// BuildPushoverURL converts PushoverConfig to Shoutrrr URL format.
// URL example: pushover://:token@user?devices=device1,device2&priority=1&title=Container+Update
func BuildPushoverURL(config PushoverConfig) (string, error) {
	user := strings.TrimSpace(config.User)
	token := strings.TrimSpace(config.Token)

	if user == "" {
		return "", errors.New("pushover user key is required")
	}
	if token == "" {
		return "", errors.New("pushover token is required")
	}
	if config.Priority < -2 || config.Priority > 2 {
		return "", errors.New("pushover priority must be between -2 and 2")
	}

	u := &url.URL{
		Scheme: "pushover",
		Host:   user,
		User:   url.UserPassword("", token),
	}

	q := u.Query()
	devices := make([]string, 0, len(config.Devices))
	for _, device := range config.Devices {
		trimmed := strings.TrimSpace(device)
		if trimmed != "" {
			devices = append(devices, trimmed)
		}
	}
	if len(devices) > 0 {
		q.Set("devices", strings.Join(devices, ","))
	}
	if config.Priority != 0 {
		q.Set("priority", strconv.FormatInt(int64(config.Priority), 10))
	}
	if title := strings.TrimSpace(config.Title); title != "" {
		q.Set("title", title)
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// SendPushover sends a message via Shoutrrr Pushover using proper service configuration.
func SendPushover(ctx context.Context, config PushoverConfig, message string) error {
	if strings.TrimSpace(config.Token) == "" {
		return errors.New("pushover token is empty")
	}
	if strings.TrimSpace(config.User) == "" {
		return errors.New("pushover user key is empty")
	}
	if config.Priority < -2 || config.Priority > 2 {
		return errors.New("pushover priority must be between -2 and 2")
	}

	shoutrrrURL, err := BuildPushoverURL(config)
	if err != nil {
		return fmt.Errorf("failed to build shoutrrr Pushover URL: %w", err)
	}

	return sendShoutrrrInternal("Pushover", shoutrrrURL, message, nil)
}
