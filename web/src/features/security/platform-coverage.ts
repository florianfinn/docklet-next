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
  return normalizePlatform(containerPlatform) === normalizePlatform(scannedPlatform) ? "covered" : "not-covered";
}

// Follows the scan side's platform normalisation (containerd platforms.Normalize):
// default variants are implied, so linux/arm equals linux/arm/v7 and linux/arm64/v8 equals linux/arm64.
export function normalizePlatform(platform: string): string {
  const [os = "", rawArch = "", rawVariant = ""] = platform.toLowerCase().split("/");
  let arch = rawArch, variant = rawVariant;
  if (arch === "aarch64") arch = "arm64";
  else if (arch === "x86_64" || arch === "x86-64") arch = "amd64";
  else if (arch === "i386") arch = "386";
  else if (arch === "armhf") { arch = "arm"; variant ||= "v7"; }
  else if (arch === "armel") { arch = "arm"; variant ||= "v6"; }
  if (arch === "arm64" && variant === "v8") variant = "";
  if (arch === "amd64" && variant === "v1") variant = "";
  if (arch === "arm" && variant === "") variant = "v7";
  return [os, arch, variant].filter(Boolean).join("/");
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
