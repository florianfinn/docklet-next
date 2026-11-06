// Source: docklet-next/issues/348
// Revision: 412ca6202d43c89db6e0b5b3947e86c7d2ab960d
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for the stack import contract.

import type { useTranslations } from "use-intl";
import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

export const IMPORT_PROBLEM_KEYS = {
  "import.invalid-reference": "stacks.import.errors.invalid-reference",
  "import.registry-not-configured": "stacks.import.errors.registry-not-configured",
  "import.not-found": "stacks.import.errors.not-found",
  "import.too-large": "stacks.import.errors.too-large",
  "import.timeout": "stacks.import.errors.timeout",
  "import.redirect-refused": "stacks.import.errors.redirect-refused",
  "import.source-unavailable": "stacks.import.errors.source-unavailable",
  "import.invalid-compose": "stacks.import.errors.invalid-compose",
  "import.unauthorized": "stacks.import.errors.unauthorized",
  "import.busy": "stacks.import.errors.busy",
} satisfies Record<Schema["StackimportProblem"]["code"], CatalogueKey>;

export const IMPORT_FOUND_KEYS = {
  "import.found.compose": "stacks.import.found.compose",
  "import.found.env-example": "stacks.import.found.env-example",
  "import.found.image-config": "stacks.import.found.image-config",
  "import.found.ports": "stacks.import.found.ports",
  "import.found.volumes": "stacks.import.found.volumes",
  "import.found.environment": "stacks.import.found.environment",
} satisfies Record<Schema["StackimportResult"]["found"][number], CatalogueKey>;

export const IMPORT_MISSING_KEYS = {
  "import.missing.env-file": "stacks.import.missing.env-file",
  "import.missing.label-file": "stacks.import.missing.label-file",
  "import.missing.include": "stacks.import.missing.include",
  "import.missing.extends": "stacks.import.missing.extends",
  "import.missing.build-context": "stacks.import.missing.build-context",
  "import.missing.variables": "stacks.import.missing.variables",
  "import.missing.compose": "stacks.import.missing.compose",
} satisfies Record<Schema["StackimportResult"]["missing"][number], CatalogueKey>;

/** Import refusals render from the contract code only; upstream text never reaches the UI. */
export function importProblemMessage(code: string | undefined, translate: ReturnType<typeof useTranslations>): string {
  const key = code !== undefined && Object.hasOwn(IMPORT_PROBLEM_KEYS, code) ? IMPORT_PROBLEM_KEYS[code as keyof typeof IMPORT_PROBLEM_KEYS] : "common.commandErrors.failed";
  return translate(key as never);
}
