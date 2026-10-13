import type { ScheduledJob } from "./model";
import { SCAN_SCHEDULE_ANCHOR } from "./scan-schedule";

/** Anchors that scheduled-job links focus on their target page. */
export const UPDATE_CHECK_ANCHOR = "update-check";
export const UPDATE_POLICIES_ANCHOR = "update-policies";
export const AUTO_UPDATE_ANCHOR = "auto-update";

/**
 * Page that configures a scheduled job, from its typed ids only: update
 * checks and scans open their hub setting, a backup its volume's backup tab
 * (the backup list without a volume id), a stack auto-update the stack's
 * update pane and other auto-updates the update policies.
 */
export function jobHref(job: Pick<ScheduledJob, "kind" | "stackId" | "volumeId">): string {
  switch (job.kind) {
    case "update-check":
      return `/settings/updates#${UPDATE_CHECK_ANCHOR}`;
    case "scan":
      return `/settings/security#${SCAN_SCHEDULE_ANCHOR}`;
    case "backup":
      return job.volumeId ? `/volumes/${encodeURIComponent(job.volumeId)}?tab=backups` : "/backups";
    case "auto-update":
      return job.stackId ? `/stacks/${encodeURIComponent(job.stackId)}?tab=overview#${AUTO_UPDATE_ANCHOR}` : `/settings/updates#${UPDATE_POLICIES_ANCHOR}`;
  }
}
