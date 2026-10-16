// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original sign-in mode vocabulary.

package auth

type SignInMode string

const (
	SignInModeNone  SignInMode = "none"
	SignInModeLocal SignInMode = "local"
	SignInModeProxy SignInMode = "proxy"
)
