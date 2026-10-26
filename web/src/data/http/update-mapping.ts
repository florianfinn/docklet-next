// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231.

import type { components } from "@/api/generated/schema";
import type { ImageVersion, UpdateInfo } from "@/domain/model";
import { observation } from "./update-validation";
import { levelFromVersions } from "@/domain/updates";

/** Observations belong to the exact configured reference; raw server errors are never UI copy. */
export function mapUpdateInfo(dto: components["schemas"]["ImageUpdateInfo"] | undefined, current: ImageVersion, reference: string, stale = false): UpdateInfo {
  if (dto) dto = observation(dto);
  if (!dto || dto.imageRef !== reference) return { state: "unchecked", current };
  const checkedAt = dto.checkedAt || undefined;
  const metadata = { ...(dto.code ? { code: dto.code } : {}), authState: dto.authState, stale };
  const image = { ...current, ...(dto.currentVersion ? { version: dto.currentVersion } : {}) };
  if (dto.state === "local") return { ...metadata, state: "local", current: image };
  if (dto.state === "unchecked") return { ...metadata, state: "unchecked", current: image };
  if (dto.state === "unknown" || !checkedAt) return { ...metadata, state: "unknown", current: image, checkedAt };
  if (dto.state === "error") return { ...metadata, state: "error", current: image, checkedAt, message: "" };
  const first = reference.split("/")[0] ?? "";
  const registry = dto.authRegistry || (reference.includes("/") && /[.:]/.test(first) ? first : "docker.io");
  if (dto.state === "current") return { ...metadata, state: "current", current: image, checkedAt, registry };
  if (dto.state === "rebuild") return { ...metadata, state: "rebuild", current: image, latest: { ...image, ...(dto.latestVersion ? { version: dto.latestVersion } : {}) }, checkedAt, registry };
  if (dto.state === "version" && /^v?\d+\.\d+\.\d+$/.test(dto.currentVersion) && /^v?\d+\.\d+\.\d+$/.test(dto.latestVersion)) {
    return { ...metadata, state: "version", current: image, latest: { ...image, tag: dto.latestVersion, version: dto.latestVersion }, level: levelFromVersions(dto.currentVersion, dto.latestVersion), checkedAt, registry };
  }
  return { ...metadata, state: "unknown", current: image, checkedAt };
}
