// Source: docklet-next/issues/586
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original setup test with snapshot-owned provider cleanup.

package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	dto "github.com/florianfinn/docklet-next/types/tailnet"
)

// MintTestKey uses the already validated token, discards the key, and returns only its cleanup ID.
// A successful response without an ID proves mint permission but cannot prove cleanup.
func (p *Provisioner) MintTestKey(ctx context.Context, token string) (string, error) {
	var payload keyRequest
	payload.Capabilities.Devices.Create.Tags = []string{AgentTag}
	payload.ExpirySeconds = 300
	payload.Description = "Docklet setup test"
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/api/v2/tailnet/-/keys", bytes.NewReader(body))
	if err != nil {
		return "", testProviderError(0)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client().Do(req)
	if err != nil {
		return "", testProviderError(0)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", testProviderError(resp.StatusCode)
	}
	data, err := readBounded(resp.Body)
	if err != nil {
		return "", testProviderError(0)
	}
	// The bearer token, key and provider text never enter an error or log; only the ID is retained.
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(data, &result) != nil {
		return "", testProviderError(0)
	}
	return result.ID, nil
}

func testProviderError(status int) *dto.Problem {
	code := dto.CodeProviderUnavailable
	if status == 400 || status == 401 || status == 403 {
		code = dto.CodeMintRefused
	}
	return &dto.Problem{Code: code, Status: http.StatusBadGateway, ProviderStatus: status}
}

// DeleteKey treats an already absent test key as cleaned up and never depends on provider error text.
func (p *Provisioner) DeleteKey(ctx context.Context, token, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, p.base()+"/api/v2/tailnet/-/keys/"+url.PathEscape(id), nil)
	if err != nil {
		return &dto.Problem{Code: dto.CodeTestCleanupFailed, Status: 502}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client().Do(req)
	if err != nil {
		return &dto.Problem{Code: dto.CodeTestCleanupFailed, Status: 502}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 || resp.StatusCode == 204 || resp.StatusCode == 404 {
		return nil
	}
	return &dto.Problem{Code: dto.CodeTestCleanupFailed, Status: 502, ProviderStatus: resp.StatusCode}
}
