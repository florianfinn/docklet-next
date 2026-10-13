// Source: getarcaneapp/arcane/backend/pkg/utils/notifications/slack_sender.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package notifications

import (
	"context"
	"errors"
	"fmt"

	"github.com/nicholas-fedor/shoutrrr/pkg/services/chat/slack"
)

// BuildSlackURL converts SlackConfig to Shoutrrr URL format using shoutrrr's Config
func BuildSlackURL(config SlackConfig) (string, error) {
	if config.Token == "" {
		return "", errors.New("slack token is required")
	}

	// Parse the token to get the token object
	token, err := slack.ParseToken(config.Token)
	if err != nil {
		return "", fmt.Errorf("invalid Slack token format (expected format: xoxb-... or xoxp-...): %w", err)
	}

	slackConfig := &slack.Config{
		BotName:  config.BotName,
		Icon:     config.Icon,
		Token:    *token,
		Color:    config.Color,
		Title:    config.Title,
		Channel:  config.Channel,
		ThreadTS: config.ThreadTS,
	}

	url := slackConfig.GetURL()
	return url.String(), nil
}

// SendSlack sends a message via Shoutrrr Slack using proper service configuration
func SendSlack(ctx context.Context, config SlackConfig, message string) error {
	if config.Token == "" {
		return errors.New("slack token is empty")
	}

	shoutrrrURL, err := BuildSlackURL(config)
	if err != nil {
		return fmt.Errorf("failed to build shoutrrr Slack URL: %w", err)
	}

	return sendShoutrrrInternal("Slack", shoutrrrURL, message, nil)
}
