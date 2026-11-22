// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original proof that tsnet log lines redact exact configured secrets (M36-S4A-R01).

package tailnet

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTSNetLogsRedactExactSecrets(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	secret := strings.Repeat("manual-example-", 3)
	node, err := NewTSNetNode(NodeConfig{Dir: t.TempDir(), Hostname: "docklet-example", AuthKey: secret, Secrets: []string{secret}})
	require.NoError(t, err)
	server := node.(*tsnetNode).server
	server.Logf("control: login failed for %s", secret)
	server.UserLogf("auth key %s and tskey-auth-example-value", secret)
	require.Contains(t, buf.String(), "login failed")
	require.NotContains(t, buf.String(), secret)
	require.NotContains(t, buf.String(), "tskey-auth-example-value")
}
