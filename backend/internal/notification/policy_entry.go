// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Existing channel fan-out carries a durable policy-entry identity and neutral delivery errors.

package notification

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/types/imageupdate"
	wire "github.com/florianfinn/docklet-next/types/notification"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

type imagePolicyEntryKey struct{}

func (s *NotificationService) NotifyImagePolicyCandidate(ctx context.Context, env, entry string, c dto.UpdateCandidate) (int, error) {
	ctx = context.WithValue(ctx, imagePolicyEntryKey{}, entry)
	response, err := s.DispatchNotificationForEnvironment(ctx, env, wire.DispatchRequest{Kind: wire.DispatchKindImageUpdate, ImageUpdate: &wire.DispatchImageUpdate{ImageRef: c.ImageRef, UpdateInfo: imageupdate.Response{HasUpdate: true, LatestVersion: c.Version, LatestDigest: c.Digest, Platform: c.Platform}}})
	return response.Delivered, err
}
func policyDeliveryError(ctx context.Context, err error) error {
	if _, ok := ctx.Value(imagePolicyEntryKey{}).(string); ok && err != nil {
		return errors.New("Notification delivery failed")
	}
	return err
}
