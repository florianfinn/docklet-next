// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original previous-image slot projection for standalone container updates.

package container

import "time"

// PreviousImage is the image a standalone container ran before its last Docklet update.
// It is present only while "Restore previous image" is offered: the slot image differs
// from the running image and the slot still belongs to the container running under the name.
type PreviousImage struct {
	ImageRef  string    `json:"imageRef" doc:"Image reference the container ran before the update"`
	ImageID   string    `json:"imageId" doc:"Local image ID kept for the restore"`
	CreatedAt time.Time `json:"createdAt" doc:"When the slot was written"`
}
