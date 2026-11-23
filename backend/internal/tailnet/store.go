// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original encrypted, write-only hub credential storage for the tailnet route.

package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"go.getarcane.app/sys/crypto"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

const (
	enabledKey    = "tailnetEnabled"
	credentialKey = "tailnetCredential"
)

// SettingsBackend is the part of the settings service the store needs.
type SettingsBackend interface {
	GetStringSetting(ctx context.Context, key, defaultValue string) string
	UpdateSettingValues(ctx context.Context, updates []libarcane.SettingUpdate) error
}

// Store persists the route switch and the hub credential, encrypted with the hub encryption key.
type Store struct {
	settings SettingsBackend
}

func NewStore(settings SettingsBackend) *Store { return &Store{settings: settings} }

// Load returns the stored switch and decrypted credential.
func (s *Store) Load(ctx context.Context) (bool, Credential, error) {
	enabled, _ := strconv.ParseBool(s.settings.GetStringSetting(ctx, enabledKey, "false"))
	sealed := s.settings.GetStringSetting(ctx, credentialKey, "")
	if sealed == "" {
		return enabled, Credential{}, nil
	}
	plain, err := crypto.Decrypt(sealed)
	if err != nil {
		return false, Credential{}, errors.New("stored tailnet credential cannot be decrypted")
	}
	var cred Credential
	if err := json.Unmarshal([]byte(plain), &cred); err != nil {
		return false, Credential{}, errors.New("stored tailnet credential is malformed")
	}
	return enabled, cred, nil
}

// Save stores both values atomically.
func (s *Store) Save(ctx context.Context, enabled bool, cred Credential) error {
	sealed := ""
	if cred.configured() {
		plain, err := json.Marshal(cred)
		if err != nil {
			return err
		}
		if sealed, err = crypto.Encrypt(string(plain)); err != nil {
			return err
		}
	}
	return s.settings.UpdateSettingValues(ctx, []libarcane.SettingUpdate{
		{Key: enabledKey, Value: strconv.FormatBool(enabled)},
		{Key: credentialKey, Value: sealed},
	})
}

// ErrInvalidCredential refuses an update whose credential fields do not match its mode.
var ErrInvalidCredential = errors.New("tailnet credential is incomplete for the selected mode")

// Merge applies an update to the stored credential: an empty secret keeps the stored one of the same mode.
func Merge(stored Credential, update tailnettypes.SettingsUpdate) (Credential, error) {
	mode := strings.TrimSpace(update.Mode)
	if mode == "" {
		mode = stored.Mode
	}
	next := Credential{Mode: mode}
	switch mode {
	case tailnettypes.ModeOAuth:
		next.ClientID = strings.TrimSpace(update.ClientID)
		next.ClientSecret = strings.TrimSpace(update.ClientSecret)
		if next.ClientSecret == "" && stored.Mode == mode {
			next.ClientSecret = stored.ClientSecret
			if next.ClientID == "" {
				next.ClientID = stored.ClientID
			}
		}
		if next.ClientID == "" || !strings.HasPrefix(next.ClientSecret, "tskey-client-") {
			return Credential{}, ErrInvalidCredential
		}
	case tailnettypes.ModeManual:
		next.AuthKey = strings.TrimSpace(update.AuthKey)
		if next.AuthKey == "" && stored.Mode == mode {
			next.AuthKey = stored.AuthKey
		}
		if next.AuthKey == "" || strings.ContainsAny(next.AuthKey, " \t\r\n") {
			return Credential{}, ErrInvalidCredential
		}
	case "":
		if update.Enabled {
			return Credential{}, ErrInvalidCredential
		}
	default:
		return Credential{}, ErrInvalidCredential
	}
	return next, nil
}

// InstallerControlURL returns the raw TAILSCALE_CONTROL_URL when agents' installers may carry it: an absolute http(s) URL with a
// host, no userinfo, quotes, backslashes or whitespace (Unicode spaces and U+FEFF, the JavaScript \s set used by
// web/src/domain/agent-install.ts installerControlUrl).
func InstallerControlURL(raw string) (string, bool) {
	if raw == "" || strings.ContainsAny(raw, "'\"\\\uFEFF") || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return "", false
	}
	return raw, true
}

// ReadModel is the write-only settings view: secrets are reported only as configured, and a control URL only when installers may
// carry it; customControl stays true for any configured value so onboarding can refuse the route instead of joining the default server.
func ReadModel(enabled bool, cred Credential, hostname, controlURL string) tailnettypes.Settings {
	usable, _ := InstallerControlURL(controlURL)
	return tailnettypes.Settings{
		Enabled:          enabled,
		Mode:             cred.Mode,
		ClientID:         cred.ClientID,
		SecretConfigured: cred.configured(),
		Hostname:         hostname,
		CustomControl:    controlURL != "",
		ControlURL:       usable,
		PolicySnippet:    PolicySnippet(),
	}
}
