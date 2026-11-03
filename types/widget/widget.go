// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original hub service widget contracts.

package widget

import "time"

type Kind string

const (
	Plex      Kind = "plex"
	Overseerr Kind = "overseerr"
	Minecraft Kind = "minecraft"
)

func (k Kind) Valid() bool { return k == Plex || k == Overseerr || k == Minecraft }

type Code string

const (
	NotConfigured         Code = "widget.not-configured"
	Disabled              Code = "widget.disabled"
	Unreachable           Code = "widget.unreachable"
	Timeout               Code = "widget.timeout"
	AuthFailed            Code = "widget.auth-failed"
	BadResponse           Code = "widget.bad-response"
	RedirectRefused       Code = "widget.redirect-refused"
	TLSFailed             Code = "widget.tls-failed"
	CredentialUnavailable Code = "widget.credential-unavailable"
	InvalidConfig         Code = "widget.invalid-config"
	PersistenceFailed     Code = "widget.persistence-failed"
)

type Problem struct {
	Code Code `json:"code" enum:"widget.not-configured,widget.disabled,widget.unreachable,widget.timeout,widget.auth-failed,widget.bad-response,widget.redirect-refused,widget.tls-failed,widget.credential-unavailable,widget.invalid-config,widget.persistence-failed"`
	// Certificate is present only on a transient administrator test result.
	Certificate *CertificatePin `json:"certificate,omitempty"`
}

func (p *Problem) Error() string { return string(p.Code) }
func (p *Problem) GetStatus() int {
	if p.Code == InvalidConfig {
		return 400
	}
	return 503
}
func Failure(code Code) *Problem { return &Problem{Code: code} }

// CertificatePin binds an exact leaf certificate to the configured authority.
type CertificatePin struct {
	SHA256   string `json:"sha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	HostPort string `json:"hostPort" maxLength:"300"`
}

type StackReference struct {
	EnvironmentID string `json:"environmentId" maxLength:"200"`
	ProjectID     string `json:"projectId" maxLength:"200"`
}

type Config struct {
	Kind          Kind            `json:"kind" enum:"plex,overseerr,minecraft"`
	Enabled       bool            `json:"enabled"`
	Address       string          `json:"address"`
	HasCredential bool            `json:"hasCredential"`
	TLSPin        *CertificatePin `json:"tlsPin,omitempty"`
	OpenURL       string          `json:"openUrl"`
	Stack         *StackReference `json:"stack,omitempty"`
	Revision      int64           `json:"revision"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// CredentialAction makes omitted secrets safe: keep never overwrites a secret.
type ConfigUpdate struct {
	Enabled          bool            `json:"enabled"`
	Address          string          `json:"address" maxLength:"2048"`
	CredentialAction string          `json:"credentialAction" enum:"keep,replace,clear" default:"keep"`
	Credential       string          `json:"credential,omitempty" maxLength:"4096" writeOnly:"true"`
	TLSPin           *CertificatePin `json:"tlsPin,omitempty"`
	OpenURL          string          `json:"openUrl" maxLength:"2048"`
	Stack            *StackReference `json:"stack,omitempty"`
}

type Summary struct {
	Kind       Kind            `json:"kind" enum:"plex,overseerr,minecraft"`
	Enabled    bool            `json:"enabled"`
	Configured bool            `json:"configured"`
	OpenURL    string          `json:"openUrl"`
	Stack      *StackReference `json:"stack,omitempty"`
}

type Result struct {
	Kind       Kind           `json:"kind" enum:"plex,overseerr,minecraft"`
	Stale      bool           `json:"stale"`
	AgeSeconds int64          `json:"ageSeconds"`
	FetchedAt  *time.Time     `json:"fetchedAt,omitempty"`
	Error      *Problem       `json:"error,omitempty"`
	Plex       *PlexData      `json:"plex,omitempty"`
	Overseerr  *OverseerrData `json:"overseerr,omitempty"`
	Minecraft  *MinecraftData `json:"minecraft,omitempty"`
}

type PlexData struct {
	Streams    int           `json:"streams"`
	Transcodes int           `json:"transcodes"`
	Sessions   []PlexSession `json:"sessions"`
}
type PlexSession struct {
	User      string  `json:"user"`
	Title     string  `json:"title"`
	Subtitle  string  `json:"subtitle"`
	Device    string  `json:"device"`
	Progress  float64 `json:"progress"`
	Quality   string  `json:"quality"`
	Transcode bool    `json:"transcode"`
}
type OverseerrData struct {
	Pending   int            `json:"pending"`
	Approved  int            `json:"approved"`
	Available int            `json:"available"`
	Requests  []MediaRequest `json:"requests"`
}
type MediaRequest struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	MediaType   string `json:"mediaType" enum:"movie,tv"`
	RequestedBy string `json:"requestedBy"`
	CreatedAt   string `json:"createdAt"`
}
type MinecraftData struct {
	Online  int      `json:"online"`
	Max     int      `json:"max"`
	MOTD    string   `json:"motd"`
	Version string   `json:"version"`
	Players []string `json:"players"`
}

type ActionResult struct {
	ActivityID string `json:"activityId"`
}
