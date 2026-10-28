// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original startup writer barrier and restore-history settlement wiring.

package volume

import "context"

func (s *VolumeService) SweepBackupWriters(ctx context.Context) error {
	return s.backup.SweepWriters(ctx)
}
func (s *VolumeService) BackupWriterFenced(name string) bool { return s.backup.WriterFenced(name) }
func (s *VolumeService) ReconcileInterruptedBackups(ctx context.Context, _ ...string) error {
	return s.backup.SettleInterrupted(ctx, false)
}
func (s *VolumeService) ReconcileRestoredBackups(ctx context.Context) error {
	return s.backup.ReconcileStartup(ctx, true)
}
func (s *VolumeService) RetryFencedBackupWriters(ctx context.Context) {
	s.backup.RetryFencedWriters(ctx)
}
