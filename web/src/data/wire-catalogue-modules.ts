// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original total module refusal catalogue map.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

export const MODULE_CODE_KEYS = {
  "module-busy": "settings.modules.codes.module-busy",
  "module-on": "settings.modules.codes.module-on",
  "operation-conflict": "settings.modules.codes.operation-conflict",
  "feature-disabled": "settings.modules.codes.feature-disabled",
  "agent.update-required": "common.commandErrors.updateRequired",
} satisfies Record<components["schemas"]["ModulesModuleProblem"]["code"], CatalogueKey>;
