// Source: docklet-next/issues/592
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original adoption entry point of the M38 contract foundation; the rule is implemented with #592.

package proxy

import (
	"context"

	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// Adopt lets an instance follow a container recreated with the identical source (docs/design/proxy-module.md, P8).
func (m *Module) Adopt(ctx context.Context, instanceID string) (dto.Adoption, error) {
	return dto.Adoption{}, dto.Fail(dto.NotImplemented, 501)
}
