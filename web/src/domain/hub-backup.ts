// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original Hub archive view model for issue #32.

import type { components } from "@/api/generated/schema";

export type HubBackupRun = components["schemas"]["BackupSystemBackupRun"];
export type HubBackupPolicy = components["schemas"]["BackupSystemBackupPolicy"];
export type HubBackupStorage = components["schemas"]["BackupSystemBackupStorage"];
export type HubBackupMaintenance = components["schemas"]["BackupSystemBackupMaintenance"];
export type HubBackupOperation = components["schemas"]["BackupSystemBackupOperation"];

export interface HubBackupAccess {
  read: boolean;
  manage: boolean;
  restore: boolean;
}

export interface HubBackupApi {
  getHubBackupAccess(): Promise<HubBackupAccess>;
  listHubBackups(): Promise<HubBackupRun[]>;
  getHubBackupPolicies(): Promise<HubBackupPolicy[]>;
  saveHubBackupPolicies(policies: HubBackupPolicy[]): Promise<HubBackupPolicy[]>;
  getHubBackupStorage(): Promise<HubBackupStorage>;
  getHubBackupMaintenance(): Promise<HubBackupMaintenance>;
  getHubBackupActivityStatus(activityId: string): Promise<string>;
  createHubBackup(): Promise<HubBackupRun>;
  uploadHubBackup(file: File, onProgress: (percent: number) => void): Promise<HubBackupOperation>;
  restoreHubBackup(id: string): Promise<HubBackupOperation>;
  deleteHubBackup(id: string): Promise<HubBackupOperation>;
  downloadHubBackup(id: string): Promise<void>;
}

export const MAX_HUB_BACKUP_BYTES = 2 * 1024 * 1024 * 1024;

export function hubBackupErrorCode(error: unknown): string {
  if (error && typeof error === "object") {
    const value = error as { code?: unknown; outcome?: { code?: unknown } };
    if (typeof value.code === "string") return value.code;
    if (typeof value.outcome?.code === "string") return value.outcome.code;
  }
  return "backup.failed";
}
