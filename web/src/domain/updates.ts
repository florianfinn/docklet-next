import type { ImageVersion, Service, Tone, UpdateInfo, UpdateLevel } from "./model";

export type UpdateVisual = "current" | "major" | "minor" | "patch" | "rebuild" | "error" | "local" | "unchecked";

/**
 * Collapses an update result into the visual category. Version levels come only
 * from release versions; a new image state for the same tag stays "rebuild"
 * and never implies bug or security fixes.
 */
export function updateVisual(update: UpdateInfo): UpdateVisual {
  switch (update.state) {
    case "version":
      return update.level;
    case "rebuild":
      return "rebuild";
    case "current":
      return "current";
    case "error":
      return "error";
    case "local":
      return "local";
    default:
      return "unchecked";
  }
}

export const UPDATE_TONE: Record<UpdateVisual, Tone> = {
  current: "ok",
  major: "orange",
  minor: "warn",
  patch: "warn",
  rebuild: "cyan",
  error: "err",
  local: "gray",
  unchecked: "gray",
};

const PRIORITY: Record<UpdateVisual, number> = {
  error: 6,
  major: 5,
  minor: 4,
  patch: 3,
  rebuild: 2,
  unchecked: 1,
  local: 0,
  current: 0,
};

export function hasUpdate(update: UpdateInfo): boolean {
  return update.state === "version" || update.state === "rebuild";
}

export interface UpdateSummary {
  pending: number;
  visual: UpdateVisual;
  errors: number;
  items: { service: string; update: UpdateInfo }[];
}

export function summarizeUpdates(services: Pick<Service, "name" | "update">[]): UpdateSummary {
  let visual: UpdateVisual = "current";
  let pending = 0;
  let errors = 0;
  for (const s of services) {
    const v = updateVisual(s.update);
    if (hasUpdate(s.update)) pending++;
    if (v === "error") errors++;
    if (PRIORITY[v] > PRIORITY[visual]) visual = v;
  }
  if (visual === "unchecked" && services.some((s) => s.update.state === "current")) visual = "current";
  return { pending, visual, errors, items: services.map((s) => ({ service: s.name, update: s.update })) };
}

/** Display label for an image state: release version first, tag second. */
export function versionLabel(image: ImageVersion): string {
  if (image.version && image.version !== image.tag) return `${image.tag} · ${image.version}`;
  return image.version ?? image.tag;
}

export function levelFromVersions(current: string, latest: string): UpdateLevel {
  const a = current.replace(/^v/, "").split(/[.-]/).map((x) => Number.parseInt(x, 10));
  const b = latest.replace(/^v/, "").split(/[.-]/).map((x) => Number.parseInt(x, 10));
  if ((b[0] ?? 0) !== (a[0] ?? 0)) return "major";
  if ((b[1] ?? 0) !== (a[1] ?? 0)) return "minor";
  return "patch";
}

export function imageRef(image: ImageVersion): string {
  return `${image.repository}:${image.tag}`;
}
