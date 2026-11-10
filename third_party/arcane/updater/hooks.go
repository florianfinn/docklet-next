// Source: getarcaneapp/kit/updater/hooks.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Decline Docklet self targets; retain identity routing without the Arcane self-updater path.

package updater

import (
	"context"
	"errors"
	"strings"
)

func (s *Service) triggerSelfUpdate(ctx context.Context, containerID, containerName, newImageRef string, labels map[string]string) error {
	return errors.New("image.system-protected: use the Docklet update action")
}

// isSelfUpdateCandidate identifies protected host containers by Docklet role,
// legacy policy or the current process container ID. Workload updates decline them.
func (s *Service) isSelfUpdateCandidate(containerID string, labels map[string]string) bool {
	if labels["io.docklet.system"] == "hub" || labels["io.docklet.system"] == "agent" || s.config.LabelPolicy.IsSelfUpdateTarget(labels) {
		return true
	}
	selfID := strings.TrimSpace(s.config.SelfContainerID)
	return selfID != "" && (strings.HasPrefix(containerID, selfID) || strings.HasPrefix(selfID, containerID))
}

func (s *Service) recordResult(ctx context.Context, result ResourceResult) error {
	if s.config.RunRecorder == nil {
		return nil
	}
	return s.config.RunRecorder.RecordUpdateRun(ctx, result)
}

func (s *Service) notify(ctx context.Context, containerID, containerName, imageRef, oldImage, newImage string) error {
	if s.config.Notifier == nil {
		return nil
	}
	return s.config.Notifier.Notify(ctx, Notification{
		ContainerID:   containerID,
		ContainerName: containerName,
		ImageRef:      imageRef,
		OldImage:      oldImage,
		NewImage:      newImage,
	})
}

// recordEvent records a container-scoped update event; every event the
// updater emits today concerns a container, so the resource type is fixed.
func (s *Service) recordEvent(ctx context.Context, phase, resourceID, resourceName string, metadata map[string]any) error {
	if s.config.EventRecorder == nil {
		return nil
	}
	return s.config.EventRecorder.RecordEvent(ctx, Event{
		Phase:        phase,
		Severity:     "info",
		ResourceID:   resourceID,
		ResourceName: resourceName,
		ResourceType: ResourceTypeContainer,
		Metadata:     metadata,
	})
}
