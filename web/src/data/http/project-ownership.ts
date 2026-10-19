// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label and explicit runtime membership projection.

import type { ProjectRow } from "./inventory-mapping";

// Match backend/pkg/projects.NormalizeProjectName and compose-go v2.16.1:
// strip non-ASCII label characters and leading separators, preserving an input
// that normalizes to empty. Display/configuration names remain unchanged.
export function normalizeProjectName(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9_-]/g, "").replace(/^[_-]+/, "") || name;
}

function effectiveName(project: ProjectRow): string {
  return normalizeProjectName(project.composeProjectName || project.name);
}

export function composeProjectOwner(projects: ProjectRow[], label: string | undefined, containerId?: string): ProjectRow | undefined {
  if (!label) return undefined;
  const candidates = projects.filter((project) => effectiveName(project) === label);
  // Current responses carry the effective label even for legacy fallback rows.
  // A differing observation outranks defaults; exact default spellings then
  // precede normalized aliases, including initial-discovery observations.
  candidates.sort((a, b) => {
    const rank = (project: ProjectRow) => project.composeProjectName && effectiveName(project) !== normalizeProjectName(project.name) ? 0 : project.name === label ? 1 : 2;
    return rank(a) - rank(b) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  });
  if (candidates.length) return candidates[0];
  // With no visible label owner, explicit backend runtime IDs retain container
  // membership (including directory fallback) for current and older peers.
  // This evidence cannot establish volume/network ownership.
  return containerId ? projects.filter((project) => project.runtimeServices?.some((service) => service.containerId === containerId)).sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0)[0] : undefined;
}
