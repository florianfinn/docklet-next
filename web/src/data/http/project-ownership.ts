// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label and explicit runtime membership projection.

import type { ProjectRow } from "./inventory-mapping";
import { representableId } from "./row-isolation";

// Match backend/pkg/projects.NormalizeProjectName and compose-go v2.16.1:
// strip non-ASCII label characters and leading separators, preserving an input
// that normalizes to empty. Display/configuration names remain unchanged.
export function normalizeProjectName(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9_-]/g, "").replace(/^[_-]+/, "") || name;
}

function effectiveName(project: ProjectRow): string {
  return normalizeProjectName(project.composeProjectName || project.name);
}

/** A project row without a representable id and a string name cannot own anything; it never affects other rows' ownership (#600). */
export function readableProject(project: ProjectRow | null | undefined): project is ProjectRow {
  return !!project && representableId(project.id) && typeof project.name === "string" && project.name.length > 0
    && (project.composeProjectName === undefined || project.composeProjectName === null || typeof project.composeProjectName === "string");
}

export function composeProjectOwner(rows: ProjectRow[], label: string | undefined, containerId?: string): ProjectRow | undefined {
  if (!label) return undefined;
  const projects = rows.filter(readableProject);
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
  return containerId ? projects.filter((project) => Array.isArray(project.runtimeServices) && project.runtimeServices.some((service) => service?.containerId === containerId)).sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0)[0] : undefined;
}
