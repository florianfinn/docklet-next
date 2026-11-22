// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original OAuth client-credentials and per-host auth key minting against the Tailscale API.

package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// DefaultAPIBase is the Tailscale API origin used for OAuth and key minting.
const DefaultAPIBase = "https://api.tailscale.com"

const (
	hostKeyExpiry    = 24 * time.Hour
	maxProviderBytes = 64 << 10
	providerTimeout  = 20 * time.Second
)

// Provisioner exchanges an OAuth client for access tokens and mints one-use host keys.
type Provisioner struct {
	BaseURL string
	Client  *http.Client
}

func (p *Provisioner) base() string {
	if p != nil && strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimRight(p.BaseURL, "/")
	}
	return DefaultAPIBase
}

func (p *Provisioner) client() *http.Client {
	if p != nil && p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: providerTimeout}
}

// rejected is a credential refusal (invalid, revoked, missing scope or tag ownership).
func rejected(status int) *tailnettypes.Problem {
	return &tailnettypes.Problem{Code: tailnettypes.CodeCredentialRejected, Status: http.StatusConflict, ProviderStatus: status}
}

// mintFailed is a provider or transport failure; a key may exist upstream but none is stored or returned.
func mintFailed(status int) *tailnettypes.Problem {
	return &tailnettypes.Problem{Code: tailnettypes.CodeKeyMintFailed, Status: http.StatusBadGateway, ProviderStatus: status}
}

func classify(status int) *tailnettypes.Problem {
	if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusBadRequest {
		return rejected(status)
	}
	return mintFailed(status)
}

func readBounded(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxProviderBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxProviderBytes {
		return nil, errors.New("provider response too large")
	}
	return data, nil
}

// AccessToken validates the OAuth client by requesting a client-credentials token.
func (p *Provisioner) AccessToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/api/v2/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", mintFailed(0)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client().Do(req)
	if err != nil {
		return "", mintFailed(0)
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := readBounded(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", classify(resp.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if readErr != nil || json.Unmarshal(data, &token) != nil || strings.TrimSpace(token.AccessToken) == "" {
		return "", mintFailed(resp.StatusCode)
	}
	return token.AccessToken, nil
}

type keyRequest struct {
	Capabilities  keyCapabilities `json:"capabilities"`
	ExpirySeconds int64           `json:"expirySeconds"`
	Description   string          `json:"description,omitempty"`
}

type keyCapabilities struct {
	Devices struct {
		Create struct {
			Reusable      bool     `json:"reusable"`
			Ephemeral     bool     `json:"ephemeral"`
			Preauthorized bool     `json:"preauthorized"`
			Tags          []string `json:"tags"`
		} `json:"create"`
	} `json:"devices"`
}

// MintHostKey returns a one-use, pre-authorised, non-ephemeral key tagged for agents that expires after one day.
func (p *Provisioner) MintHostKey(ctx context.Context, clientID, clientSecret string) (string, error) {
	token, err := p.AccessToken(ctx, clientID, clientSecret)
	if err != nil {
		return "", err
	}
	var payload keyRequest
	payload.Capabilities.Devices.Create.Preauthorized = true
	payload.Capabilities.Devices.Create.Tags = []string{AgentTag}
	payload.ExpirySeconds = int64(hostKeyExpiry / time.Second)
	payload.Description = "Docklet Edge host"
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/api/v2/tailnet/-/keys", bytes.NewReader(body))
	if err != nil {
		return "", mintFailed(0)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client().Do(req)
	if err != nil {
		return "", mintFailed(0)
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := readBounded(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", classify(resp.StatusCode)
	}
	var key struct {
		Key string `json:"key"`
	}
	if readErr != nil || json.Unmarshal(data, &key) != nil || !strings.HasPrefix(key.Key, "tskey-") {
		return "", mintFailed(resp.StatusCode)
	}
	return key.Key, nil
}

// HubAuthKey is the tsnet AuthKey for the hub: the OAuth secret resolves to a persistent, pre-authorised node key.
func HubAuthKey(mode, clientSecret, authKey string) string {
	if mode == tailnettypes.ModeOAuth {
		return strings.TrimSpace(clientSecret) + "?ephemeral=false&preauthorized=true"
	}
	return strings.TrimSpace(authKey)
}

// PolicySnippet is the ACL fragment the operator pastes: tag ownership and one grant agent → hub ingress port.
func PolicySnippet() string {
	return fmt.Sprintf(`{
  "tagOwners": {
    %q: ["autogroup:admin"],
    %q: ["autogroup:admin"]
  },
  "grants": [
    {"src": [%q], "dst": [%q], "ip": ["tcp:%s"]}
  ]
}`, HubTag, AgentTag, AgentTag, HubTag, IngressPort)
}
