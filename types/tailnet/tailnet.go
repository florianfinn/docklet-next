// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original embedded Tailscale route contract for Edge hosts (hub-side DTOs only). M37 T1: absent provider statuses stay off the wire under encoding/json/v2.

// Package tailnet holds the hub-side wire vocabulary of the optional Tailscale route.
package tailnet

import "time"

const (
	CodeMintRefused             = "tailnet.mint-refused"
	CodeTestCleanupFailed       = "tailnet.test-cleanup-failed"
	CodeTestRunning             = "tailnet.test-running"
	CodeCredentialRejected      = "tailnet.credential-rejected"
	CodeCoordinationUnreachable = "tailnet.coordination-unreachable"
	CodeNotConnected            = "tailnet.not-connected"
	CodeKeyMintFailed           = "tailnet.key-mint-failed"
	CodeProviderUnavailable     = "tailnet.provider-unavailable"
	CodeMTLSUnsupported         = "tailnet.mtls-unsupported"
	// CodeTagsNotPermitted: the provider refused the requested tags for this credential (#599); RefusedTags names them.
	CodeTagsNotPermitted = "tailnet.tags-not-permitted"
)

const (
	RouteLAN     = "lan"
	RouteDomain  = "domain"
	RouteTailnet = "tailnet"
)

const (
	ModeOAuth  = "oauth"
	ModeManual = "manual"
)

// Problem is a typed refusal of a tailnet operation.
type Problem struct {
	Code   string `json:"code" enum:"tailnet.credential-rejected,tailnet.coordination-unreachable,tailnet.not-connected,tailnet.key-mint-failed,tailnet.provider-unavailable,tailnet.mtls-unsupported,tailnet.mint-refused,tailnet.test-cleanup-failed,tailnet.test-running,tailnet.tags-not-permitted"`
	Status int    `json:"status"`
	// ProviderStatus is the HTTP status the Tailscale API answered with, when one was received.
	ProviderStatus int `json:"providerStatus,omitzero"`
	// RefusedTags are the tags the provider refused (tailnet.tags-not-permitted); tag names only.
	RefusedTags []string `json:"refusedTags,omitzero" maxItems:"8" pattern:"^tag:[a-z0-9-]+$"`
}

func (p *Problem) Error() string             { return p.Code }
func (p *Problem) GetStatus() int            { return p.Status }
func (p *Problem) ContentType(string) string { return "application/problem+json" }

// Connection is the hub node's observed tailnet membership; link 1 of the host chain uses only this.
type Connection struct {
	State string `json:"state" enum:"off,joining,connected,control-unreachable,error"`
	Code  string `json:"code,omitempty" enum:"tailnet.credential-rejected,tailnet.coordination-unreachable,tailnet.not-connected,tailnet.mtls-unsupported,tailnet.tags-not-permitted"`
	// RefusedTags are the tags the control server refused for the hub join (tailnet.tags-not-permitted).
	RefusedTags []string `json:"refusedTags,omitzero" maxItems:"8" pattern:"^tag:[a-z0-9-]+$"`
}

// Provisioning is the credential's ability to admit new hosts, independent of the connection.
type Provisioning struct {
	State string `json:"state" enum:"not-configured,not-applicable,checking,ok,rejected,unavailable"`
	Code  string `json:"code,omitempty" enum:"tailnet.credential-rejected,tailnet.provider-unavailable,tailnet.tags-not-permitted"`
	// ProviderStatus is the last rejecting HTTP status from the Tailscale API.
	ProviderStatus int      `json:"providerStatus,omitzero"`
	RefusedTags    []string `json:"refusedTags,omitzero" maxItems:"8" pattern:"^tag:[a-z0-9-]+$"`
}

// Status is the redacted hub tailnet status; it never carries keys or peer endpoints.
type Status struct {
	Enabled      bool         `json:"enabled"`
	Mode         string       `json:"mode,omitempty" doc:"oauth or manual"`
	Connection   Connection   `json:"connection"`
	Provisioning Provisioning `json:"provisioning"`
	TailnetName  string       `json:"tailnetName,omitempty"`
	HubAddress   string       `json:"hubAddress,omitempty" doc:"The hub's tailnet IPv4 address"`
	HostCount    int          `json:"hostCount"`
	OnlineHosts  int          `json:"onlineHosts"`
	// RouteAvailable reports whether onboarding may offer the tailnet route for new hosts.
	RouteAvailable bool `json:"routeAvailable"`
	// RouteCode explains why the route is unavailable.
	RouteCode          string `json:"routeCode,omitempty" doc:"tailnet.* refusal code"`
	Generation         int64  `json:"generation"`
	CredentialRevision string `json:"credentialRevision"`
}

// HostPeer is the hub's view of one tailnet-route host.
type HostPeer struct {
	State      string     `json:"state" enum:"online,offline,unknown"`
	LastSeen   *time.Time `json:"lastSeen,omitempty"`
	Path       string     `json:"path" doc:"direct, relay, idle or unknown"`
	Tagged     *bool      `json:"tagged,omitempty" doc:"Whether the observed node carries tag:docklet-agent"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
	// FailingLink is the first failing link of the hub → peer → tunnel chain, or none.
	FailingLink string `json:"failingLink" doc:"hub, peer, tunnel or none"`
	// DNSName is the node's MagicDNS name without the trailing dot; Addresses are its tailnet IPs, IPv4 first (#601).
	DNSName   string   `json:"dnsName,omitzero" maxLength:"253"`
	Addresses []string `json:"addresses,omitzero" maxItems:"4"`
}

// Settings is the write-only credential read model of Settings → Connections.
type Settings struct {
	Enabled          bool   `json:"enabled"`
	Mode             string `json:"mode,omitempty" doc:"oauth or manual"`
	ClientID         string `json:"clientId,omitempty"`
	SecretConfigured bool   `json:"secretConfigured"`
	Hostname         string `json:"hostname"`
	CustomControl    bool   `json:"customControl" doc:"TAILSCALE_CONTROL_URL is set"`
	ControlURL       string `json:"controlUrl,omitempty" doc:"Self-hosted control server (TAILSCALE_CONTROL_URL) that new agents join; written into their installer"`
	PolicySnippet    string `json:"policySnippet"`
}

// SettingsUpdate replaces the hub credential; an empty secret keeps the stored one for the same mode.
type SettingsUpdate struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode,omitempty" enum:"oauth,manual"`
	ClientID     string `json:"clientId,omitempty" maxLength:"256"`
	ClientSecret string `json:"clientSecret,omitempty" maxLength:"512"`
	AuthKey      string `json:"authKey,omitempty" maxLength:"512"`
}

// TestResult records only the checks proved by this request, never credentials.
type TestResult struct {
	CredentialRevision string      `json:"credentialRevision"`
	Stale              bool        `json:"stale"`
	Checks             []TestCheck `json:"checks" nullable:"false"`
}

// TestCheck separates provider admission from hub membership and cleanup certainty.
type TestCheck struct {
	ID             string   `json:"id" enum:"credential,agent-key,cleanup,hub-joined,hub-tag,hub-address"`
	State          string   `json:"state" enum:"pass,fail,skipped,unknown"`
	Code           string   `json:"code,omitempty" enum:"tailnet.credential-rejected,tailnet.provider-unavailable,tailnet.not-connected,tailnet.mint-refused,tailnet.test-cleanup-failed,tailnet.test-running,tailnet.tags-not-permitted"`
	ProviderStatus int      `json:"providerStatus,omitzero"`
	RefusedTags    []string `json:"refusedTags,omitzero" maxItems:"8" pattern:"^tag:[a-z0-9-]+$"`
}
