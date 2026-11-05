// Source: docklet-next/issues/265
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-container scan platform coverage.

import type { ContainerInfo, ScannedImage } from "@/domain/model";

export type PlatformCoverage = "covered" | "not-covered" | "unknown";

/**
 * Compares a container's image platform with the image's last completed scan.
 * A platform the daemon does not report is unknown coverage, never covered or not.
 */
export function platformCoverage(containerPlatform: string | undefined, scannedPlatform: string | undefined): PlatformCoverage | undefined {
  if (!scannedPlatform) return undefined;
  if (!containerPlatform || containerPlatform === "unknown") return "unknown";
  return normalize(containerPlatform) === normalize(scannedPlatform) ? "covered" : "not-covered";
}

// Docker reports arm64 with or without its default v8 variant.
function normalize(platform: string): string {
  return platform.toLowerCase().replace(/^([^/]+)\/arm64\/v8$/, "$1/arm64");
}

/** Containers of one scanned image (same host-qualified image ID) grouped by coverage. */
export function imageCoverage(image: Pick<ScannedImage, "id" | "hostId" | "platform" | "scannedAt">, containers: readonly Pick<ContainerInfo, "name" | "hostId" | "imageId" | "imagePlatform">[]): Record<Exclude<PlatformCoverage, "covered">, string[]> {
  const out = { "not-covered": [] as string[], unknown: [] as string[] };
  if (!image.scannedAt) return out;
  for (const c of containers) {
    if (c.hostId !== image.hostId || !image.id || c.imageId !== image.id) continue;
    const state = platformCoverage(c.imagePlatform, image.platform);
    if (state === "not-covered" || state === "unknown") out[state].push(c.name);
  }
  return out;
}
