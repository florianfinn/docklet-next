// Source: docklet-next/issues/584
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import type { components } from "@/api/generated/schema";
import type { MessageCode } from "../message-code";

type Activity = components["schemas"]["ActivityActivity"];

export const TYPE_TITLES: Readonly<Record<string, string>> = {
  "hub.update": "hubUpdate",
  "agent.update": "agentUpdate",
  "job_run": "job",
  "resource_action": "action",
  "load_sustained": "activity.runKind.load-alert",
  "image_pull": "activity.runKind.pull",
  "image_update_check": "activity.runKind.check-updates",
  "image_patch": "imagePatch",
  "project_pull": "projectPull",
  "project_build": "projectBuild",
  "project_update": "activity.runKind.update",
  "project_restore": "activity.runKind.restore",
  "project_deploy": "activity.runKind.deploy",
  "project_redeploy": "activity.runKind.redeploy",
  "project_down": "activity.runKind.stop",
  "project_restart": "activity.runKind.restart",
  "project_destroy": "projectDestroy",
  "container_start": "activity.runKind.start",
  "container_stop": "activity.runKind.stop",
  "container_restart": "activity.runKind.restart",
  "container_redeploy": "activity.runKind.redeploy",
  "container_edit": "activity.runKind.recreate",
  "container_delete": "containerDelete",
  "container_kill": "containerKill",
  "container_pause": "containerPause",
  "container_unpause": "containerUnpause",
  "container_auto_heal": "activity.runKind.auto-heal",
  "vulnerability_scan": "activity.runKind.scan",
  "vulnerability.candidate-scan": "candidateScan",
  "auto_update": "activity.runKind.update",
  "system_prune": "activity.runKind.prune",
};

export const JOB_TITLES: Readonly<Record<string, string>> = {
  "auto-heal": "autoHeal",
  "auto-update": "autoUpdate",
  "scheduled-prune": "scheduledPrune",
  "auto-patch": "autoPatch",
  "vulnerability-scan": "scan",
  "vulnerability-intel": "intel",
  "vulnerability-notify": "vulnerabilityNotify",
  "history-retention": "historyRetention",
  "history-sample": "historySample",
  "host-availability": "hostAvailability",
  "event-cleanup": "eventCleanup",
  "activity-sweep": "activitySweep",
  "expired-sessions-cleanup": "sessionCleanup",
  "upload-sessions-cleanup": "uploadCleanup",
  "docker-client-refresh": "dockerRefresh",
  "pruning-volume-helper": "volumeHelper",
  "update-check": "activity.runKind.check-updates",
  "notification-deliver": "notificationDeliver",
  "notification-delivery-wake": "notificationWake",
  "image-polling": "imagePolling",
};

export const JOB_PREFIX_TITLES: Readonly<Record<string, string>> = {
  "environment-health:": "hostAvailability",
  "update-window:": "updateWindow",
  "system-backup:": "systemBackup",
  "volume-backup:": "volumeBackup",
};

export const ACTION_TITLES: Readonly<Record<string, string>> = {
  "check_environment_connection": "connectionCheck",
  "sync_environment": "syncHost",
  "create_network": "networkCreate",
  "update_network": "networkUpdate",
  "remove_network": "networkRemove",
  "prune_networks": "networkPrune",
  "connect_network": "networkConnect",
  "disconnect_network": "networkDisconnect",
  "create_volume": "volumeCreate",
  "rename_volume": "volumeRename",
  "remove_volume": "volumeRemove",
  "prune_volumes": "volumePrune",
  "update_volume_workspace": "volumeWorkspace",
  "create_project": "projectCreate",
  "update_project": "projectSave",
  "update_project_workspace": "projectWorkspace",
  "update_tags": "projectTags",
  "create_volume_backup": "volumeBackup",
  "delete_volume_backup": "backupDelete",
  "download_volume_backup": "backupDownload",
  "restore_volume_backup": "backupRestore",
  "restore_volume_backup_files": "backupRestoreFiles",
  "update_volume_backup_policy": "backupPolicy",
  "check_volume_backup_repository": "repositoryCheck",
  "scheduled_volume_backup": "volumeBackup",
  "backup_download": "backupDownload",
  "approve": "requestApprove",
  "decline": "requestDecline",
};

/** Resource actions without an action discriminator: a structured operation or the resource kind names them. */
export const OPERATION_TITLES: Readonly<Record<string, string>> = { "container.shell": "containerShell" };
export const RESOURCE_TITLES: Readonly<Record<string, string>> = { "system_backup": "systemBackup" };

const title = (key: string, params: MessageCode["params"] = {}): MessageCode => ({ code: key.startsWith("activity.") ? key : `runMessages.activityTitles.${key}`, params });
const readable = (name: string) => name.replace(/[_.:-]+/g, " ").trim();
const fallback = (name: string) => title("fallback", { name: readable(name) });

/** Titles describe the producer operation; ActionKind continues to control behaviour. */
export function activityTitle(run: Activity): MessageCode {
  if (run.type === "hub.update" && typeof run.metadata?.targetVersion === "string" && run.metadata.targetVersion.trim()) {
    return title("hubUpdateVersion", { version: run.metadata.targetVersion });
  }
  if (run.type === "resource_action") {
    const action = typeof run.metadata?.action === "string" ? run.metadata.action : "";
    if (action) return ACTION_TITLES[action] ? title(ACTION_TITLES[action]) : fallback(action);
    const operation = typeof run.metadata?.operation === "string" ? run.metadata.operation : "";
    const key = OPERATION_TITLES[operation] ?? (run.resourceType ? RESOURCE_TITLES[run.resourceType] : undefined);
    return key ? title(key) : title("action");
  }
  if (run.type === "job_run") {
    const job = typeof run.metadata?.jobId === "string" ? run.metadata.jobId : run.resourceId ?? "";
    const key = JOB_TITLES[job] ?? Object.entries(JOB_PREFIX_TITLES).find(([prefix]) => job.startsWith(prefix))?.[1];
    return key ? title(key) : job ? fallback(job) : title("job");
  }
  return TYPE_TITLES[run.type] ? title(TYPE_TITLES[run.type]!) : fallback(run.type);
}
