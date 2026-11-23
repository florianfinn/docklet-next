// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original write-only credential storage, read model and log redaction tests.

package tailnet_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

var secret = tailnettest.Key("client", "kExample-SECRETVALUE")

type memorySettings struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memorySettings) GetStringSetting(_ context.Context, key, def string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.values[key]; ok && v != "" {
		return v
	}
	return def
}

func (m *memorySettings) UpdateSettingValues(_ context.Context, updates []libarcane.SettingUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range updates {
		m.values[u.Key] = u.Value
	}
	return nil
}

func initCrypto(t *testing.T) {
	t.Helper()
	crypto.InitEncryption(&crypto.Config{EncryptionKey: "hex:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Environment: "test"})
}

func TestCredentialIsEncryptedAndWriteOnly(t *testing.T) {
	initCrypto(t)
	settings := &memorySettings{values: map[string]string{}}
	store := tailnet.NewStore(settings)
	next, err := tailnet.Merge(tailnet.Credential{}, tailnettypes.SettingsUpdate{Enabled: true, Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: secret})
	require.NoError(t, err)
	require.NoError(t, store.Save(t.Context(), true, next))
	require.NotContains(t, settings.values["tailnetCredential"], "SECRETVALUE", "the stored value is encrypted")

	enabled, loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.True(t, enabled)
	require.Equal(t, secret, loaded.ClientSecret)

	read, err := json.Marshal(tailnet.ReadModel(enabled, loaded, "docklet-hub", ""))
	require.NoError(t, err)
	require.NotContains(t, string(read), "SECRETVALUE")
	require.Contains(t, string(read), `"secretConfigured":true`)
	require.NotContains(t, string(read), "controlUrl")
	// A self-hosted control server is reported so new agents' installers join it.
	custom := tailnet.ReadModel(enabled, loaded, "docklet-hub", "https://control.example.com")
	require.True(t, custom.CustomControl)
	require.Equal(t, "https://control.example.com", custom.ControlURL)
	// A value installers must not carry is never serialised; customControl still makes onboarding refuse the route.
	for _, unusable := range []string{"https://example-user:" + "example-pass@control.example.com", "control.example.com", "ftp://control.example.com", "https://control.example.com/'x"} {
		refused := tailnet.ReadModel(enabled, loaded, "docklet-hub", unusable)
		body, err := json.Marshal(refused)
		require.NoError(t, err)
		require.True(t, refused.CustomControl, unusable)
		require.Empty(t, refused.ControlURL, unusable)
		require.NotContains(t, string(body), "example-pass")
	}

	// An empty secret keeps the stored one (rotation is an explicit new value).
	kept, err := tailnet.Merge(loaded, tailnettypes.SettingsUpdate{Enabled: true, Mode: tailnettypes.ModeOAuth})
	require.NoError(t, err)
	require.Equal(t, secret, kept.ClientSecret)
	rotated, err := tailnet.Merge(loaded, tailnettypes.SettingsUpdate{Enabled: true, Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: tailnettest.Key("client", "kExample-ROTATED")})
	require.NoError(t, err)
	require.Equal(t, "tskey-client-kExample-ROTATED", rotated.ClientSecret)

	_, err = tailnet.Merge(loaded, tailnettypes.SettingsUpdate{Enabled: true, Mode: tailnettypes.ModeManual})
	require.ErrorIs(t, err, tailnet.ErrInvalidCredential, "switching mode needs the new mode's key")
	_, err = tailnet.Merge(tailnet.Credential{}, tailnettypes.SettingsUpdate{Enabled: true, Mode: tailnettypes.ModeOAuth, ClientID: "c", ClientSecret: tailnettest.Key("auth", "not-a-client")})
	require.ErrorIs(t, err, tailnet.ErrInvalidCredential)
}

func TestStatusAndLogsNeverCarryTheCredential(t *testing.T) {
	initCrypto(t)
	logs := &lockedBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(errors.New("tsnet.Up: error resolving auth key: 401 Unauthorized for " + secret + "?ephemeral=false"))
	m := tailnet.NewManager(tailnet.ManagerConfig{StateDir: filepath.Join(t.TempDir(), "ts"), MTLSDisabled: true, Factory: fleet.Factory, Provisioner: &tailnet.Provisioner{BaseURL: "http://127.0.0.1:1"}})
	t.Cleanup(func() { m.Close(context.Background()) })
	m.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: secret})
	require.Eventually(t, func() bool { return m.Status().Connection.Code == tailnettypes.CodeCredentialRejected }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, secret+"?ephemeral=false&preauthorized=true", fleet.Nodes()[0].Config.AuthKey)
	require.Equal(t, []string{tailnet.HubTag}, fleet.Nodes()[0].Config.AdvertiseTags, "the hub advertises its tag only in OAuth mode")

	status, err := json.Marshal(m.Status())
	require.NoError(t, err)
	require.NotContains(t, string(status), "SECRETVALUE")
	require.NotEmpty(t, logs.String())
	require.NotContains(t, logs.String(), "SECRETVALUE")
	require.Equal(t, "dial tskey-[redacted] now", tailnet.Redact("dial tskey-auth-kAbc-DEF123?preauthorized=true now"))
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestManualModeDoesNotAdvertiseTags(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := tailnet.NewManager(tailnet.ManagerConfig{StateDir: filepath.Join(t.TempDir(), "ts"), MTLSDisabled: true, Factory: fleet.Factory})
	t.Cleanup(func() { m.Close(context.Background()) })
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return len(fleet.Nodes()) == 1 }, 2*time.Second, 5*time.Millisecond)
	require.Empty(t, fleet.Nodes()[0].Config.AdvertiseTags)
	require.Equal(t, manual.AuthKey, fleet.Nodes()[0].Config.AuthKey)
}

// The raw TAILSCALE_CONTROL_URL decides configuredness and installer use: whitespace anywhere (incl. Unicode spaces) withholds it.
func TestRawControlURLDecidesInstallerUse(t *testing.T) {
	initCrypto(t)
	for _, raw := range []string{" https://control.example.com", "https://control.example.com\n", "https://control.example.com/a\u00a0b", "https://control.example.com/\ufeff", " ", "https://:443", "http://:3552"} {
		service := tailnet.NewService(&config.Config{TailscaleControlURL: raw, EdgeMTLSMode: "disabled", TailscaleStateDir: t.TempDir()}, &memorySettings{values: map[string]string{}})
		settings, err := service.Settings(t.Context())
		require.NoError(t, err)
		require.True(t, settings.CustomControl, "%q", raw)
		require.Empty(t, settings.ControlURL, "%q", raw)
	}
	service := tailnet.NewService(&config.Config{TailscaleControlURL: "https://control.example.com", EdgeMTLSMode: "disabled", TailscaleStateDir: t.TempDir()}, &memorySettings{values: map[string]string{}})
	settings, err := service.Settings(t.Context())
	require.NoError(t, err)
	require.Equal(t, "https://control.example.com", settings.ControlURL)
}
