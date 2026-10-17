// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package remenv

import "context"

// AgentAccessCheck binds a forwarded operation to the current host credential.
type AgentAccessCheck func(context.Context, string, *string, string, string) (context.Context, context.CancelFunc, error)
