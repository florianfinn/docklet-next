// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

export const BACKUP_CODE_KEYS = {
  "volume.exists": "volumes.backupCodes.volume_exists",
  "volume.not-found": "volumes.backupCodes.volume_not-found",
  "volume.backup.busy": "volumes.backupCodes.volume_backup_busy",
  "volume.backup.interrupted": "volumes.backupCodes.volume_backup_interrupted",
  "volume.backup.restart-failed": "volumes.backupCodes.volume_backup_restart-failed",
  "volume.backup.retention-failed": "volumes.backupCodes.volume_backup_retention-failed",
  "volume.backup.policy-single": "volumes.backupCodes.volume_backup_policy-single",
  "volume.backup.policy-invalid": "volumes.backupCodes.volume_backup_policy-invalid",
  "volume.backup.repository-unavailable": "volumes.backupCodes.volume_backup_repository-unavailable",
  "volume.backup.engine-unavailable": "volumes.backupCodes.volume_backup_engine-unavailable",
  "volume.backup.engine-timeout": "volumes.backupCodes.volume_backup_engine-timeout",
  "volume.backup.engine-output-invalid": "volumes.backupCodes.volume_backup_engine-output-invalid",
  "volume.backup.repository-check-failed": "volumes.backupCodes.volume_backup_repository-check-failed",
  "volume.backup.upload-too-large": "volumes.backupCodes.volume_backup_upload-too-large",
  "volume.backup.upload-invalid": "volumes.backupCodes.volume_backup_upload-invalid",
  "volume.backup.download-too-large": "volumes.backupCodes.volume_backup_download-too-large",
  "volume.backup.listing-too-large": "volumes.backupCodes.volume_backup_listing-too-large",
  "volume.backup.snapshot-not-found": "volumes.backupCodes.volume_backup_snapshot-not-found",
  "volume.backup.safety-pinned": "volumes.backupCodes.volume_backup_safety-pinned",
  "volume.backup.writer-unconfirmed": "volumes.backupCodes.volume_backup_writer-unconfirmed",
  "volume.restore.self-mounted": "volumes.backupCodes.volume_restore_self-mounted",
  "volume.restore.safety-failed": "volumes.backupCodes.volume_restore_safety-failed",
  "volume.restore.failed-partial": "volumes.backupCodes.volume_restore_failed-partial",
  "volume.restore.interrupted": "volumes.backupCodes.volume_restore_interrupted",
  "volume.restore.interrupted-before-write": "volumes.backupCodes.volume_restore_interrupted-before-write",
  "volume.restore.restart-failed": "volumes.backupCodes.volume_restore_restart-failed",
  "backup.target.path-missing": "volumes.backupCodes.backup_target_path-missing",
  "backup.target.not-mounted": "volumes.backupCodes.backup_target_not-mounted",
  "backup.target.permission-denied": "volumes.backupCodes.backup_target_permission-denied",
  "backup.target.mount-failed": "volumes.backupCodes.backup_target_mount-failed",
  "backup.target.unreachable": "volumes.backupCodes.backup_target_unreachable",
  "backup.target.unknown": "volumes.backupCodes.backup_target_unknown",
  "backup.target.wrong-host": "volumes.backupCodes.backup_target_wrong-host",
  "backup.target.retired": "volumes.backupCodes.backup_target_retired",
  "backup.target.in-use": "volumes.backupCodes.backup_target_in-use",
  "backup.target.hosts-unknown": "volumes.backupCodes.backup_target_hosts-unknown",
  "backup.target.duplicate": "volumes.backupCodes.backup_target_duplicate",
  "backup.target.invalid": "volumes.backupCodes.backup_target_invalid",
  "backup.target.name-exists": "volumes.backupCodes.backup_target_name-exists",
  "backup.target.stale-set": "volumes.backupCodes.backup_target_stale-set",
  "backup.target.key-mismatch": "volumes.backupCodes.backup_target_key-mismatch",
  "backup.target.sync-failed": "volumes.backupCodes.backup_target_sync-failed",
  "backup.target.conflict": "volumes.backupCodes.backup_target_conflict",
  "backup.target.immutable": "volumes.backupCodes.backup_target_immutable",
  "volume.bind.invalid-key": "volumes.backupCodes.volume_bind_invalid-key",
  "volume.bind.path-denied": "volumes.backupCodes.volume_bind_path-denied",
  "volume.bind.not-mounted": "volumes.backupCodes.volume_bind_not-mounted",
  "volume.bind.source-missing": "volumes.backupCodes.volume_bind_source-missing",
  "volume.bind.unsupported-type": "volumes.backupCodes.volume_bind_unsupported-type",
  "volume.bind.unsupported": "volumes.backupCodes.volume_bind_unsupported",
  "volume.bind.file-operation-unsupported": "volumes.backupCodes.volume_bind_file-operation-unsupported",
  "volume.bind.recovery-failed": "volumes.backupCodes.volume_bind_recovery-failed",
  "volume.bind.snapshot-kind-mismatch": "volumes.backupCodes.volume_bind_snapshot-kind-mismatch",
  "volume.bind.size-timeout": "volumes.backupCodes.volume_bind_size-timeout",
  "volume.helper-unavailable": "volumes.backupCodes.volume_helper-unavailable",
  "volume.operation-failed": "volumes.backupCodes.volume_operation-failed",
  "volume.workspace-busy": "volumes.backupCodes.volume_workspace-busy",
  "backup.copy.mount-failed": "volumes.backupCodes.backup_copy_mount-failed",
  "backup.copy.write-failed": "volumes.backupCodes.backup_copy_write-failed",
  "backup.copy.insufficient-space": "volumes.backupCodes.backup_copy_insufficient-space",
  "runtime.interrupted": "volumes.backupCodes.runtime_interrupted",
  "backup.target.probe-host-mismatch": "volumes.backupCodes.backup_target_probe-host-mismatch",
  "backup.target.location-conflict": "volumes.backupCodes.backup_target_location-conflict",
  "backup.target.identity-unavailable": "volumes.backupCodes.backup_target_identity-unavailable",
} satisfies Record<NonNullable<
  | Schema["BackupProblem"]["code"]
  | Schema["VolumeHelperUnavailableError"]["code"]
>, CatalogueKey>;

export const BACKUP_REASON_KEYS = {
  "format": "volumes.uploadReasons.format",
  "path": "volumes.uploadReasons.path",
  "duplicate": "volumes.uploadReasons.duplicate",
  "entry-type": "volumes.uploadReasons.entry-type",
  "link": "volumes.uploadReasons.link",
  "too-many-entries": "volumes.uploadReasons.too-many-entries",
  "expanded-too-large": "volumes.uploadReasons.expanded-too-large",
  "insufficient-space": "volumes.uploadReasons.insufficient-space",
  "system": "volumes.uploadReasons.system",
  "docker": "volumes.uploadReasons.docker",
  "docklet": "volumes.uploadReasons.docklet",
} satisfies Record<NonNullable<
  | Schema["BackupProblem"]["reason"]
  | Schema["VolumeRestoreRun"]["reason"]
>, CatalogueKey>;

export const VOLUME_COMMAND_KEYS = {
  "volume.exists": "volumes.errors.exists",
  "volume.helper-unavailable": "volumes.errors.helper-unavailable",
  "volume.helper-incompatible": "volumes.errors.helper-incompatible",
} satisfies Record<NonNullable<
  | Schema["VolumeExistsError"]["code"]
  | Schema["VolumeHelperIncompatibleError"]["code"]
  | Schema["VolumeHelperUnavailableError"]["code"]
>, CatalogueKey>;
