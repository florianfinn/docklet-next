// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original operation identity for confirmed backup writer cleanup.

package rustic

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
)

const OperationVolumeLabel = "docklet.backup-volume"
const OperationIDLabel = "docklet.backup-operation"

type operationKey struct{}
type operation struct{ volume, id string }

func WithOperation(ctx context.Context, volume, id string) context.Context {
	return context.WithValue(ctx, operationKey{}, operation{volume, id})
}
func OperationLabels(ctx context.Context) map[string]string {
	labels := volumehelper.Labels()
	if value, ok := ctx.Value(operationKey{}).(operation); ok {
		labels[OperationVolumeLabel] = value.volume
		labels[OperationIDLabel] = value.id
	}
	return labels
}
