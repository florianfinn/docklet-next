// Source: docklet-next/issues/354
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original notification link base resolver.

package notification

import (
	"context"
	"regexp"
	"strings"
)

// LinkSettings reads the general link settings; the settings service satisfies it.
type LinkSettings interface {
	GetStringSetting(ctx context.Context, key, defaultValue string) string
}

// UseLinkSettings connects the settings service once at startup; without it links use APP_URL.
func (s *NotificationService) UseLinkSettings(settings LinkSettings) { s.linkSettings = settings }

var schemePattern = regexp.MustCompile(`(?i)^https?://`)

// resolveLinkBase follows the web preview rule: the domain unless links are set to internal, else the
// internal hub address (http:// added to a bare address), else APP_URL; trailing slashes removed.
func resolveLinkBase(linkBase, domainURL, hubAddress, appURL string) string {
	if domain := strings.TrimRight(strings.TrimSpace(domainURL), "/"); domain != "" && linkBase != "internal" {
		return domain
	}
	if address := strings.TrimRight(strings.TrimSpace(hubAddress), "/"); address != "" {
		if !schemePattern.MatchString(address) {
			address = "http://" + address
		}
		return address
	}
	return strings.TrimRight(appURL, "/")
}

// LinkBase is the base URL of every hub link in notifications.
func (s *NotificationService) LinkBase(ctx context.Context) string {
	appURL := ""
	if s.config != nil {
		appURL = s.config.GetAppURL()
	}
	if s.linkSettings == nil {
		return strings.TrimRight(appURL, "/")
	}
	return resolveLinkBase(s.linkSettings.GetStringSetting(ctx, "linkBase", "auto"), s.linkSettings.GetStringSetting(ctx, "domainUrl", ""), s.linkSettings.GetStringSetting(ctx, "hubAddress", ""), appURL)
}
