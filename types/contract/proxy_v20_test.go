// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original v20 fingerprint, provider table and legacy gate contracts.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestM28ProxyV20Fixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/agent-hub-v21.json")
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	require.Equal(t, AgentHubFingerprint, hex.EncodeToString(sum[:]))
	var manifest struct {
		Version int
		Proxy   struct {
			MinimumContract int
			DNSProviders    []dto.DNSProviderDefinition
			Reasons         []string
		}
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, 21, manifest.Version)
	require.Equal(t, 20, manifest.Proxy.MinimumContract)
	require.Equal(t, dto.DNSProviders(), manifest.Proxy.DNSProviders)
	require.Contains(t, manifest.Proxy.Reasons, "source-ineligible")
	require.Contains(t, manifest.Proxy.Reasons, "unknown-operation-kind")
	legacy, err := os.ReadFile("testdata/agent-hub-v19.json")
	require.NoError(t, err)
	sum = sha256.Sum256(legacy)
	require.Equal(t, AgentHubV19Fingerprint, hex.EncodeToString(sum[:]))
	require.True(t, AgentCompatible(19, AgentHubV19Fingerprint))
	require.False(t, ModuleCompatible(19, AgentHubV19Fingerprint))
	require.False(t, ModuleCompatible(19, AgentHubFingerprint))
}
func TestM28RedactedResolverDigest(t *testing.T) {
	first := "example-live-value-one"
	second := "example-live-value-two"
	q := dto.ResolverRequest{ResolverConfig: dto.ResolverConfig{Name: "test", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "letsencrypt"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}}, Variables: []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &first}}}
	intent := q.Redact()
	digest, err := intent.Digest()
	require.NoError(t, err)
	q.Variables[0].Value = &second
	same, err := q.Redact().Digest()
	require.NoError(t, err)
	require.Equal(t, digest, same)
	raw, err := json.Marshal(intent)
	require.NoError(t, err)
	require.NotContains(t, string(raw), first)
	require.NotContains(t, string(raw), "value")
	require.Contains(t, string(raw), `"mode":"new"`)
	q.Variables[0].Value = nil
	reuse, err := q.Redact().Digest()
	require.NoError(t, err)
	require.NotEqual(t, digest, reuse)
}
