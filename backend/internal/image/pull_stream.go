// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package image

import (
	"github.com/danielgtaylor/huma/v2"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"reflect"
)

// PullFrame describes one JSON line, not an array or an HTTP response envelope.
// An activity frame precedes progress; creation failure emits only an error.
// Error and done are exclusive terminals.
type PullFrame struct{}

func (PullFrame) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{OneOf: []*huma.Schema{
		r.Schema(reflect.TypeFor[imageDTO.PullStartedFrame](), true, ""),
		r.Schema(reflect.TypeFor[imageDTO.PullProgressFrame](), true, ""),
		r.Schema(reflect.TypeFor[imageDTO.PullErrorFrame](), true, ""),
		r.Schema(reflect.TypeFor[imageDTO.PullDoneFrame](), true, ""),
	}}
}
