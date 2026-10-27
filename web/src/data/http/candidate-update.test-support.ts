// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Sanitized producer-shaped candidate update fixtures for issue #231.

import type { components } from "@/api/generated/schema";
import type { ProjectRow } from "./inventory-mapping";

export const candidateAt = "2026-10-27T00:00:00.123456Z";
export const candidateObservation = (imageRef = "docker.io/library/busybox:1.35.0"): components["schemas"]["ImageUpdateInfo"] => ({ state: "version", checkedAt: candidateAt, checkTime: candidateAt, imageRef, platform: "linux/amd64", authState: "unknown", currentVersion: "1.35.0", latestVersion: "1.38.0", currentDigest: `sha256:${"a".repeat(64)}`, latestDigest: `sha256:${"b".repeat(64)}`, updateType: "minor", error: "", responseTimeMs: 0, hasUpdate: true, usedCredential: false });
export function candidateUpdateInfo(reference = "busybox:1.35.0", observed = "docker.io/library/busybox:1.35.0"): NonNullable<ProjectRow["updateInfo"]> {
  const info = candidateObservation(observed);
  return { serviceUpdates: { app: { imageRef: reference, updateInfo: info } }, updateInfoByRef: { [reference]: info }, status: "has_update", hasUpdate: true, imageCount: 1, checkedImageCount: 1, imagesWithUpdates: 1, imagesNotPulled: 0, errorCount: 0, lastCheckedAt: candidateAt };
}
export const candidateProject: ProjectRow = { id: "project", name: "Example", path: "/srv/projects/example", status: "running", serviceCount: 1, runningCount: 1, isArchived: false, tags: [], createdAt: candidateAt, updatedAt: candidateAt, runtimeServices: [{ name: "app", image: "busybox:1.35.0", status: "running" }] };
