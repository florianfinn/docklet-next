import type { Issue } from "@/domain/compose/validate";

export type SectionKey = "general" | "ports" | "volumes" | "env" | "health" | "networks" | "extras";

const SECTION_FIELD: Partial<Record<Issue["field"], SectionKey>> = { image: "general", ports: "ports", mounts: "volumes", env: "env", health: "health" };

export function sectionFor(issue: Issue): SectionKey {
  return SECTION_FIELD[issue.field] ?? "general";
}
