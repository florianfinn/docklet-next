// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original fatal cause for actor-host loss.

package di

import "errors"

// ErrActorHostLost is the cancel cause of the application context when the embedded actor host ends unexpectedly.
var ErrActorHostLost = errors.New("actor host lost")

// AppFailure ends the application lifecycle with a fatal cause, so the process exits and its restart policy recovers it.
type AppFailure func(error)
