// Source: getarcaneapp/arcane/backend/pkg/utils/notifications/config.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Canonical provider wire types; provider config components exported from the wire vocabulary.

package notification

const (
	NotificationEventImageUpdate        EventType = "image_update"
	NotificationEventContainerUpdate    EventType = "container_update"
	NotificationEventVulnerabilityFound EventType = "vulnerability_found"
	NotificationEventPruneReport        EventType = "prune_report"
	NotificationEventAutoHeal           EventType = "auto_heal"
	NotificationEventBackupFailed       EventType = "backup_failed"
	NotificationEventDeployFailed       EventType = "deploy_failed"
	NotificationEventHostOffline        EventType = "host_offline"

	TLSModeNone     TLSMode = "none"
	TLSModeStartTLS TLSMode = "starttls"
	TLSModeSSL      TLSMode = "ssl"

	AuthModeNone    AuthMode = "none"
	AuthModeAuto    AuthMode = "auto"
	AuthModePlain   AuthMode = "plain"
	AuthModeLogin   AuthMode = "login"
	AuthModeCRAMMD5 AuthMode = "crammd5"
)

var validProviders = map[Provider]struct{}{
	NotificationProviderDiscord:    {},
	NotificationProviderEmail:      {},
	NotificationProviderTelegram:   {},
	NotificationProviderSignal:     {},
	NotificationProviderSlack:      {},
	NotificationProviderNtfy:       {},
	NotificationProviderPushover:   {},
	NotificationProviderGotify:     {},
	NotificationProviderMatrix:     {},
	NotificationProviderGoogleChat: {},
	NotificationProviderGeneric:    {},
}

func IsValidProvider(provider Provider) bool {
	_, ok := validProviders[provider]
	return ok
}

type EventType string

var AllEventTypes = []EventType{
	NotificationEventImageUpdate,
	NotificationEventContainerUpdate,
	NotificationEventVulnerabilityFound,
	NotificationEventPruneReport,
	NotificationEventAutoHeal,
	NotificationEventBackupFailed, NotificationEventDeployFailed, NotificationEventHostOffline,
}

type TLSMode string

type AuthMode string

type ConfigDiscord struct {
	WebhookID string             `json:"webhookId"`
	Token     string             `json:"token"`
	Username  string             `json:"username,omitempty"`
	AvatarURL string             `json:"avatarUrl,omitempty"`
	Events    map[EventType]bool `json:"events,omitempty"`
}

type ConfigEmail struct {
	SMTPHost     string             `json:"smtpHost"`
	SMTPPort     int                `json:"smtpPort"`
	SMTPUsername string             `json:"smtpUsername"`
	SMTPPassword string             `json:"smtpPassword"`
	FromAddress  string             `json:"fromAddress"`
	ToAddresses  []string           `json:"toAddresses"`
	TLSMode      TLSMode            `json:"tlsMode"`
	AuthMode     AuthMode           `json:"authMode,omitempty"`
	Events       map[EventType]bool `json:"events,omitempty"`
}

type ConfigTelegram struct {
	BotToken     string             `json:"botToken"`
	ChatIDs      []string           `json:"chatIds"`
	Preview      bool               `json:"preview"`
	Notification bool               `json:"notification"`
	ParseMode    string             `json:"parseMode,omitempty"`
	Title        string             `json:"title,omitempty"`
	Events       map[EventType]bool `json:"events,omitempty"`
}

type ConfigSignal struct {
	Host       string             `json:"host"`
	Port       int                `json:"port"`
	User       string             `json:"user,omitempty"`
	Password   string             `json:"password,omitempty"`
	Token      string             `json:"token,omitempty"`
	Source     string             `json:"source"`
	Recipients []string           `json:"recipients"`
	DisableTLS bool               `json:"disableTls"`
	NotifySelf *bool              `json:"notifySelf,omitempty"`
	Events     map[EventType]bool `json:"events,omitempty"`
}

type ConfigSlack struct {
	Token    string             `json:"token"`
	BotName  string             `json:"botName,omitempty"`
	Icon     string             `json:"icon,omitempty"`
	Color    string             `json:"color,omitempty"`
	Title    string             `json:"title,omitempty"`
	Channel  string             `json:"channel,omitempty"`
	ThreadTS string             `json:"threadTs,omitempty"`
	Events   map[EventType]bool `json:"events,omitempty"`
}

type ConfigNtfy struct {
	Host                   string             `json:"host"`
	Port                   int                `json:"port"`
	Topic                  string             `json:"topic"`
	Username               string             `json:"username,omitempty"`
	Password               string             `json:"password,omitempty"`
	Title                  string             `json:"title,omitempty"`
	Priority               string             `json:"priority,omitempty"`
	Tags                   []string           `json:"tags,omitempty"`
	Icon                   string             `json:"icon,omitempty"`
	Cache                  bool               `json:"cache"`
	Firebase               bool               `json:"firebase"`
	DisableTLS             bool               `json:"disableTls"`
	DisableTLSVerification bool               `json:"disableTlsVerification"`
	Events                 map[EventType]bool `json:"events,omitempty"`
}

type ConfigPushover struct {
	Token    string             `json:"token"`
	User     string             `json:"user"`
	Devices  []string           `json:"devices,omitempty"`
	Priority int8               `json:"priority"`
	Title    string             `json:"title,omitempty"`
	Events   map[EventType]bool `json:"events,omitempty"`
}

type ConfigGotify struct {
	Host               string             `json:"host"`
	Port               int                `json:"port,omitempty"`
	Token              string             `json:"token"`
	Path               string             `json:"path,omitempty"`
	Priority           int                `json:"priority,omitempty"`
	Title              string             `json:"title,omitempty"`
	DisableTLS         bool               `json:"disableTls"`
	InsecureSkipVerify bool               `json:"insecureSkipVerify"`
	UseHeader          bool               `json:"useHeader"`
	Events             map[EventType]bool `json:"events,omitempty"`
}

type ConfigGoogleChat struct {
	// WebhookURL is the full incoming webhook URL copied from Google Chat,
	// The entire URL is treated as a scalar credential.
	WebhookURL string             `json:"webhookUrl"`
	Events     map[EventType]bool `json:"events,omitempty"`
}

type ConfigMatrix struct {
	Host                   string             `json:"host"`
	Port                   int                `json:"port,omitempty"`
	Rooms                  string             `json:"rooms"`
	Username               string             `json:"username,omitempty"`
	Password               string             `json:"password,omitempty"`
	DisableTLSVerification bool               `json:"disableTlsVerification"`
	Events                 map[EventType]bool `json:"events,omitempty"`
}

type ConfigGeneric struct {
	WebhookURL    string             `json:"webhookUrl"`
	Method        string             `json:"method,omitempty"`
	ContentType   string             `json:"contentType,omitempty"`
	TitleKey      string             `json:"titleKey,omitempty"`
	MessageKey    string             `json:"messageKey,omitempty"`
	CustomHeaders map[string]string  `json:"customHeaders,omitempty"`
	DisableTLS    bool               `json:"disableTls"`
	Events        map[EventType]bool `json:"events,omitempty"`
	// SuccessBodyContains is an optional substring that must appear in the
	// response body for the send to be considered successful. Useful for
	// providers that always return HTTP 200 but embed a status indicator in
	// the JSON body (e.g. PushPlus returns {"code":200,...} on success and
	// {"code":900,...} on failure). When empty, only the HTTP status code is
	// checked (existing behaviour).
	SuccessBodyContains string `json:"successBodyContains,omitempty"`
	// PayloadTemplate is an optional Go text/template rendered into the request
	// body, letting users target endpoints that require a nested or
	// provider-specific payload shape rather than the flat
	// {"title":...,"message":...} object. The template receives the title and
	// message (under the configured TitleKey/MessageKey) plus event variables
	// (environment, environmentId, event, timestamp), all pre-escaped for JSON
	// string contexts. When empty, the flat JSON payload is used (existing
	// behaviour).
	PayloadTemplate string `json:"payloadTemplate,omitempty"`
}
