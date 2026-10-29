// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notifications

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

func invalid(field string) error {
	return &dto.Problem{Code: "notification.config-invalid", Field: "/config/" + field, Status: 400}
}

var dnsLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

func bareHost(s string) bool {
	if s == "" || len(s) > 253 || strings.ContainsAny(s, "/@?# \t\r\n") || strings.Contains(s, "://") {
		return false
	}
	if net.ParseIP(s) != nil {
		return true
	}
	if strings.Contains(s, ":") {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(s, "."), ".") {
		if len(part) > 63 || !dnsLabel.MatchString(part) {
			return false
		}
	}
	return true
}
func absoluteHTTP(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

// ValidateConfig validates the effective plaintext config after credential preservation.
func ValidateConfig(p NotificationProvider, c database.JSON) error {
	var target any
	switch p {
	case NotificationProviderDiscord:
		target = &DiscordConfig{}
	case NotificationProviderEmail:
		target = &EmailConfig{}
	case NotificationProviderTelegram:
		target = &TelegramConfig{}
	case NotificationProviderSignal:
		target = &SignalConfig{}
	case NotificationProviderSlack:
		target = &SlackConfig{}
	case NotificationProviderNtfy:
		target = &NtfyConfig{}
	case NotificationProviderPushover:
		target = &PushoverConfig{}
	case NotificationProviderGotify:
		target = &GotifyConfig{}
	case NotificationProviderMatrix:
		target = &MatrixConfig{}
	case NotificationProviderGoogleChat:
		target = &GoogleChatConfig{}
	case NotificationProviderGeneric:
		target = &GenericConfig{}
	default:
		return invalid("provider")
	}
	b, e := json.Marshal(c)
	if e != nil {
		return invalid("")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if e = dec.Decode(target); e != nil {
		field := ""
		var te *json.UnmarshalTypeError
		if errors.As(e, &te) {
			field = te.Field
		}
		if strings.HasPrefix(e.Error(), "json: unknown field ") {
			field = strings.Trim(e.Error()[20:], `"`)
		}
		return invalid(field)
	}
	typ := reflect.TypeOf(target).Elem()
	for k, v := range c {
		known := false
		for i := 0; i < typ.NumField(); i++ {
			if strings.Split(typ.Field(i).Tag.Get("json"), ",")[0] == k {
				known = true
				break
			}
		}
		if !known {
			return invalid(k)
		}
		cap := 4096
		if k == "payloadTemplate" {
			cap = 16384
		}
		if s, ok := v.(string); ok && len(s) > cap {
			return invalid(k)
		}
	}
	// Round-trip to uniform JSON values also checks direct service callers' typed maps.
	var normalized map[string]any
	_ = json.Unmarshal(b, &normalized)
	if v, ok := normalized["events"]; ok && v != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return invalid("events")
		}
		for k, v := range m {
			if !slices.Contains(AllNotificationEventTypes, NotificationEventType(k)) {
				return invalid("events/" + k)
			}
			if _, ok := v.(bool); !ok {
				return invalid("events/" + k)
			}
		}
	}
	required := func(fields ...string) error {
		for _, f := range fields {
			v, _ := normalized[f].(string)
			if strings.TrimSpace(v) == "" {
				return invalid(f)
			}
		}
		return nil
	}
	host := func(f string, optional bool) error {
		v, _ := normalized[f].(string)
		if optional && v == "" {
			return nil
		}
		if !bareHost(v) {
			return invalid(f)
		}
		return nil
	}
	port := func(f string, optional bool) error {
		v, _ := normalized[f].(float64)
		if optional && v == 0 {
			return nil
		}
		if v < 1 || v > 65535 {
			return invalid(f)
		}
		return nil
	}
	list := func(f string) error {
		v, ok := normalized[f].([]any)
		if !ok || len(v) == 0 || len(v) > 20 {
			return invalid(f)
		}
		for _, x := range v {
			s, ok := x.(string)
			if !ok || strings.TrimSpace(s) == "" || len(s) > 512 {
				return invalid(f)
			}
		}
		return nil
	}
	switch v := target.(type) {
	case *DiscordConfig:
		if e := required("webhookId", "token"); e != nil {
			return e
		}
		if v.AvatarURL != "" && !absoluteHTTP(v.AvatarURL) {
			return invalid("avatarUrl")
		}
	case *EmailConfig:
		if e := host("smtpHost", false); e != nil {
			return e
		}
		if e := port("smtpPort", false); e != nil {
			return e
		}
		if e := list("toAddresses"); e != nil {
			return e
		}
		if _, e := mail.ParseAddress(v.FromAddress); e != nil {
			return invalid("fromAddress")
		}
		for _, a := range v.ToAddresses {
			if _, e := mail.ParseAddress(a); e != nil {
				return invalid("toAddresses")
			}
		}
		if !slices.Contains([]EmailTLSMode{EmailTLSModeNone, EmailTLSModeSSL, EmailTLSModeStartTLS}, v.TLSMode) {
			return invalid("tlsMode")
		}
		if !slices.Contains([]EmailAuthMode{"", EmailAuthModeNone, EmailAuthModeAuto, EmailAuthModePlain, EmailAuthModeLogin, EmailAuthModeCRAMMD5}, v.AuthMode) {
			return invalid("authMode")
		}
		if v.AuthMode != EmailAuthModeNone && (v.SMTPUsername != "" || v.SMTPPassword != "") {
			if e := required("smtpUsername", "smtpPassword"); e != nil {
				return e
			}
		}
	case *TelegramConfig:
		if e := required("botToken"); e != nil {
			return e
		}
		if e := list("chatIds"); e != nil {
			return e
		}
		if !slices.Contains([]string{"", "HTML", "Markdown", "MarkdownV2"}, v.ParseMode) {
			return invalid("parseMode")
		}
	case *SignalConfig:
		if e := host("host", false); e != nil {
			return e
		}
		if e := port("port", false); e != nil {
			return e
		}
		if e := required("source"); e != nil {
			return e
		}
		if e := list("recipients"); e != nil {
			return e
		}
		basic := v.User != "" && v.Password != ""
		if (!basic && v.Token == "") || (v.Token != "" && (v.User != "" || v.Password != "")) {
			return invalid("token")
		}
	case *SlackConfig:
		return required("token")
	case *NtfyConfig:
		if e := host("host", true); e != nil {
			return e
		}
		if e := port("port", true); e != nil {
			return e
		}
		return required("topic")
	case *PushoverConfig:
		if e := required("token", "user"); e != nil {
			return e
		}
		if v.Priority < -2 || v.Priority > 2 {
			return invalid("priority")
		}
	case *GotifyConfig:
		if e := host("host", false); e != nil {
			return e
		}
		if e := port("port", true); e != nil {
			return e
		}
		return required("token")
	case *MatrixConfig:
		if e := host("host", false); e != nil {
			return e
		}
		if e := port("port", true); e != nil {
			return e
		}
		if e := required("rooms", "username", "password"); e != nil {
			return e
		}
		if len(strings.Split(v.Rooms, ",")) > 20 {
			return invalid("rooms")
		}
	case *GoogleChatConfig:
		if !absoluteHTTP(v.WebhookURL) {
			return invalid("webhookUrl")
		}
	case *GenericConfig:
		if !absoluteHTTP(v.WebhookURL) {
			return invalid("webhookUrl")
		}
		if v.Method != "" && !slices.Contains([]string{"POST", "PUT", "PATCH", "GET"}, v.Method) {
			return invalid("method")
		}
		if len(v.CustomHeaders) > 20 {
			return invalid("customHeaders")
		}
		for k, val := range v.CustomHeaders {
			if len(k) > 128 || len(val) > 4096 || strings.ContainsAny(k+val, "\r\n") {
				return invalid("customHeaders")
			}
		}
		if e := ValidateGenericPayloadTemplate(*v); e != nil {
			return invalid("payloadTemplate")
		}
	}
	return nil
}

// FormatDeliveryError deliberately logs only classification; provider diagnostics may embed credentials.
func FormatDeliveryError(err error) string { return fmt.Sprint(ClassifyDeliveryError(err)) }
