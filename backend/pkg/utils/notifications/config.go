// Source: getarcaneapp/arcane/backend/pkg/utils/notifications/config.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Canonical provider wire types; sender aliases retained; notification and Auto Heal contracts and delivery for #296.

package notifications

import wire "github.com/florianfinn/docklet-next/types/notification"

type NotificationProvider = wire.Provider
type NotificationEventType = wire.EventType
type EmailTLSMode = wire.TLSMode
type EmailAuthMode = wire.AuthMode
type DiscordConfig = wire.ConfigDiscord
type EmailConfig = wire.ConfigEmail
type TelegramConfig = wire.ConfigTelegram
type SignalConfig = wire.ConfigSignal
type SlackConfig = wire.ConfigSlack
type NtfyConfig = wire.ConfigNtfy
type PushoverConfig = wire.ConfigPushover
type GotifyConfig = wire.ConfigGotify
type GoogleChatConfig = wire.ConfigGoogleChat
type MatrixConfig = wire.ConfigMatrix
type GenericConfig = wire.ConfigGeneric

const (
	NotificationProviderDiscord         = wire.NotificationProviderDiscord
	NotificationProviderEmail           = wire.NotificationProviderEmail
	NotificationProviderTelegram        = wire.NotificationProviderTelegram
	NotificationProviderSignal          = wire.NotificationProviderSignal
	NotificationProviderSlack           = wire.NotificationProviderSlack
	NotificationProviderNtfy            = wire.NotificationProviderNtfy
	NotificationProviderPushover        = wire.NotificationProviderPushover
	NotificationProviderGotify          = wire.NotificationProviderGotify
	NotificationProviderMatrix          = wire.NotificationProviderMatrix
	NotificationProviderGoogleChat      = wire.NotificationProviderGoogleChat
	NotificationProviderGeneric         = wire.NotificationProviderGeneric
	NotificationEventImageUpdate        = wire.NotificationEventImageUpdate
	NotificationEventContainerUpdate    = wire.NotificationEventContainerUpdate
	NotificationEventVulnerabilityFound = wire.NotificationEventVulnerabilityFound
	NotificationEventPruneReport        = wire.NotificationEventPruneReport
	NotificationEventAutoHeal           = wire.NotificationEventAutoHeal
	NotificationEventBackupFailed       = wire.NotificationEventBackupFailed
	NotificationEventDeployFailed       = wire.NotificationEventDeployFailed
	NotificationEventHostOffline        = wire.NotificationEventHostOffline
	EmailTLSModeNone                    = wire.TLSModeNone
	EmailTLSModeStartTLS                = wire.TLSModeStartTLS
	EmailTLSModeSSL                     = wire.TLSModeSSL
	EmailAuthModeNone                   = wire.AuthModeNone
	EmailAuthModeAuto                   = wire.AuthModeAuto
	EmailAuthModePlain                  = wire.AuthModePlain
	EmailAuthModeLogin                  = wire.AuthModeLogin
	EmailAuthModeCRAMMD5                = wire.AuthModeCRAMMD5
)

var AllNotificationEventTypes = wire.AllEventTypes

func IsValidNotificationProvider(p NotificationProvider) bool { return wire.IsValidProvider(p) }
