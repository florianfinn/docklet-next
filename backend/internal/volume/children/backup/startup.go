// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original confirmed writer sweep, whole-volume fence and no-replay settlement.

package backup

import (
	"context"
	"github.com/moby/moby/api/types/mount"
	"strings"
	"time"
	"uuid"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
)

func (s *Service) isWriterFenced(name string) bool {
	s.fenceMu.Lock()
	defer s.fenceMu.Unlock()
	return s.writerDiscoveryPending || s.sweepStarted && !s.settledInitial || s.writerFences[name]
}
func (s *Service) WriterFenced(name string) bool { return s.isWriterFenced(name) }
func (s *Service) hasWriterFence(name string) bool {
	s.fenceMu.Lock()
	defer s.fenceMu.Unlock()
	return s.writerFences[name]
}
func (s *Service) SetInputCleaner(clean func(context.Context, []string) error) {
	s.inputCleaner = clean
}

// SweepWriters keeps SQL recovery evidence until inspect confirms each writer
// absent. A successful remove reply alone cannot authorize a consumer restart.
func (s *Service) SweepWriters(ctx context.Context) error {
	s.sweepMu.Lock()
	defer s.sweepMu.Unlock()
	s.fenceMu.Lock()
	s.writerDiscoveryPending = true
	s.fenceMu.Unlock()
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil
	}
	containers, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil
	}
	s.fenceMu.Lock()
	initial := !s.sweepStarted
	recovering := map[string]bool{}
	for name := range s.writerFences {
		recovering[name] = true
	}
	s.fenceMu.Unlock()
	s.fenceMu.Lock()
	if s.writerIDs == nil {
		s.writerIDs = map[string]map[string]bool{}
	}
	for _, candidate := range containers.Items {
		name := candidate.Labels[rustic.OperationVolumeLabel]
		if name == "" && candidate.Labels[volumehelper.ContainerLabel] == "true" {
			legacyEngine := candidate.Image == rustic.DefaultImage || strings.HasPrefix(candidate.Command, "/rustic ") || strings.HasPrefix(candidate.Command, "rustic ")
			legacyArchive := strings.Contains(candidate.Command, "/backups/") && strings.Contains(candidate.Command, "tar ")
			for _, mounted := range candidate.Mounts {
				if mounted.Type == mount.TypeVolume && mounted.Destination == "/volume" && mounted.Name != "" && (legacyEngine || legacyArchive || legacyDownloadVolume(mounted.Name)) {
					name = mounted.Name
					break
				}
			}
		}
		if name == "" || (!initial && !recovering[name]) {
			continue
		}
		if s.writerIDs[name] == nil {
			s.writerIDs[name] = map[string]bool{}
		}
		s.writerIDs[name][candidate.ID] = true
	}
	known := map[string][]string{}
	for name, ids := range s.writerIDs {
		for id := range ids {
			known[name] = append(known[name], id)
		}
	}
	s.fenceMu.Unlock()
	blocked := map[string]bool{}
	for name, ids := range known {
		for _, id := range ids {
			_, _ = cli.ContainerRemove(ctx, id, volumehelper.RemoveOptions())
			_, inspectErr := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
			if !errdefs.IsNotFound(inspectErr) {
				blocked[name] = true
			}
		}
	}
	s.fenceMu.Lock()
	if s.confirmedWriters == nil {
		s.confirmedWriters = map[string]bool{}
	}
	for name := range recovering {
		s.confirmedWriters[name] = !blocked[name]
	}
	for name := range known {
		if !blocked[name] {
			s.confirmedWriters[name] = true
		}
	}
	for name := range blocked {
		s.writerFences[name] = true
		s.confirmedWriters[name] = false
	}
	s.sweepStarted = true
	s.fenceMu.Unlock()

	volumes, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return err
	}
	for _, candidate := range volumes.Items {
		legacyDownload := legacyDownloadVolume(candidate.Name)
		if (candidate.Labels[restore.RestoreLabel] == "" && !legacyDownload) || candidate.Labels[volumehelper.ContainerLabel] != "true" {
			continue
		}
		name := candidate.Labels[rustic.OperationVolumeLabel]
		if legacyDownload {
			name = candidate.Name
		}
		if blocked[name] || (!initial && !recovering[name]) {
			continue
		}
		// Only operation scratch is internal-labelled; new user volumes carry the
		// restore ID alone and are removed only by CleanupNew's exact ownership check.
		_, _ = cli.VolumeRemove(ctx, candidate.Name, client.VolumeRemoveOptions{})
	}
	s.fenceMu.Lock()
	s.writerDiscoveryPending = false
	s.fenceMu.Unlock()
	return nil
}
func (s *Service) SettleInterrupted(ctx context.Context, restored bool) error {
	s.fenceMu.Lock()
	if s.writerDiscoveryPending {
		s.fenceMu.Unlock()
		return nil
	}
	initial := !s.settledInitial || restored
	eligible := map[string]bool{}
	for name, confirmed := range s.confirmedWriters {
		if confirmed {
			eligible[name] = true
		}
	}
	blocked := func(name string) bool {
		return (!initial && !eligible[name]) || (s.hasWriterFence(name) && !eligible[name])
	}
	s.fenceMu.Unlock()
	var rows []volume.Backup
	if err := s.deps.DB.WithContext(ctx).Table("volume_backups").Where("status = ?", "running").Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		if blocked(row.VolumeName) {
			continue
		}
		if !restored {
			if err := s.restartConsumers(ctx, row.StoppedContainers, user.SystemUser); err != nil {
				return err
			}
		}
		row.Status = "failed"
		row.ErrorCode = "volume.backup.interrupted"
		row.Error = row.ErrorCode
		if err := s.deps.Store.Save(ctx, row); err != nil {
			return err
		}
	}
	if err := s.lifecycle.Reconcile(ctx, restored, blocked); err != nil {
		return err
	}
	if s.inputCleaner != nil {
		var running []string
		if err := s.deps.DB.WithContext(ctx).Model(&restore.Run{}).Where("status = ?", "running").Pluck("id", &running).Error; err != nil {
			return err
		}
		if err := s.inputCleaner(ctx, running); err != nil {
			return err
		}
	}
	s.fenceMu.Lock()
	s.settledInitial = true
	for name := range eligible {
		delete(s.writerFences, name)
		delete(s.writerIDs, name)
		delete(s.confirmedWriters, name)
		if release := s.writerFenceRelease[name]; release != nil {
			release()
			delete(s.writerFenceRelease, name)
		}
	}
	s.fenceMu.Unlock()
	if s.deps.Engine != nil {
		return s.deps.Engine.SettleVolumeRuns(ctx, s.WriterFenced)
	}
	return nil
}
func (s *Service) RetryFencedWriters(ctx context.Context) {
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_ = s.ReconcileStartup(ctx, false)
		}
	}
}
func (s *Service) ReconcileStartup(ctx context.Context, restored bool) error {
	if !s.recoveryMu.TryLock() {
		return nil
	}
	defer s.recoveryMu.Unlock()
	if restored {
		s.sweepStarted = false
	}
	if err := s.SweepWriters(ctx); err != nil {
		return err
	}
	return s.SettleInterrupted(ctx, restored)
}

func legacyDownloadVolume(name string) bool {
	suffix, ok := strings.CutPrefix(name, "arcane-rustic-download-")
	if !ok {
		return false
	}
	_, err := uuid.Parse(suffix)
	return err == nil
}
