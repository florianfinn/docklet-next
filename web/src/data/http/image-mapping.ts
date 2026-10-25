// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import type { components } from "@/api/generated/schema";
import type { DockerImage, ImagePullProgress, ImageUpdateObservation } from "@/domain/images";

type Schema = components["schemas"];
export function mapImageObservation(row: Schema["ImageSummary"]): Pick<DockerImage, "protectionReasons" | "usageState" | "updateObservation"> {
  const update = row.updateInfo;
  const states = ["unchecked", "current", "version", "rebuild", "local", "unknown", "error"] as const;
  const observation: ImageUpdateObservation = update && states.includes(update.state) ? {
    state: update.state, checkedAt: update.checkedAt || undefined, currentVersion: update.currentVersion || undefined,
    candidateVersion: update.latestVersion || undefined, code: update.code,
  } : { state: "unchecked" };
  const validProtection = row.protectionReasons == null || Array.isArray(row.protectionReasons) && row.protectionReasons.every((item) => typeof item?.reason === "string");
  const protectionReasons = validProtection ? [...(row.protectionReasons ?? [])] : [];
  if (row.inUse && !protectionReasons.some((item) => item.reason === "in-use")) protectionReasons.push({ reason: "in-use" });
  return { protectionReasons, usageState: validProtection && row.usageState === "known" ? "known" : "unknown", updateObservation: observation };
}

const PROGRESS: Record<Schema["ImagePullProgressFrame"]["status"], ImagePullProgress["phase"]> = {
  "Pulling image": "pulling", "Pulling fs layer": "pulling", Waiting: "waiting", Downloading: "downloading", "Verifying Checksum": "verifying", "Download complete": "complete", Extracting: "extracting", "Pull complete": "complete", "Already exists": "complete",
};
export function mapPullProgress(row: Record<string, unknown>): ImagePullProgress | undefined {
  if (typeof row.status !== "string" || !Object.hasOwn(PROGRESS, row.status)) return undefined;
  const phase = PROGRESS[row.status as keyof typeof PROGRESS];
  const detail = row.progressDetail as Partial<Schema["ImagePullProgressCounts"]> | undefined;
  const counts = detail && Number.isFinite(detail.current) && Number.isFinite(detail.total) && detail.current! >= 0 && detail.total! >= 0 ? { current: detail.current, total: detail.total } : {};
  return { phase, ...counts };
}
