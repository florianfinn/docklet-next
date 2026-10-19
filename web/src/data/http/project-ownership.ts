// Source: docklet-next/docs/design/runtime-control.md
// Revision: 793b7fb08935a2e1b83d65aa6e2354a30330cd46d3ac6894b8d13d82938497cf
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label consequence regression coverage.

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
  // A differing observation outranks a default alias, independent of row order.
  candidates.sort((a, b) => {
    const observed = (project: ProjectRow) => Boolean(project.composeProjectName && effectiveName(project) !== normalizeProjectName(project.name));
    return Number(observed(b)) - Number(observed(a)) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  });
  if (candidates.length) return candidates[0];
  // Older peers omit the observation. Explicit runtime container IDs can then
  // link that container only; they cannot establish volume/network ownership.
  return containerId ? projects.filter((project) => !project.composeProjectName && project.runtimeServices?.some((service) => service.containerId === containerId)).sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0)[0] : undefined;
}
