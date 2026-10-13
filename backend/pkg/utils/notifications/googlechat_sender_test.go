// Source: getarcaneapp/arcane/backend/pkg/utils/notifications/googlechat_sender_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package notifications

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGoogleChatURL(t *testing.T) {
	gotURL, err := BuildGoogleChatURL(GoogleChatConfig{
		WebhookURL: "https://chat.googleapis.com/v1/spaces/FOO/messages?key=bar&token=baz",
	})
	require.NoError(t, err)
	assert.Equal(t, "googlechat://chat.googleapis.com/v1/spaces/FOO/messages?key=bar&token=baz", gotURL)
}
