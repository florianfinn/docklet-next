// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original optional-module web definitions and setup choices.

import type { components } from "@/api/generated/schema";

export const MODULES = [{ id: "proxy", settingKey: "moduleProxyEnabled" }] as const;
export type ModuleID = (typeof MODULES)[number]["id"];
export type ModuleChoices = Record<ModuleID, boolean>;
export type ModuleState = components["schemas"]["ModulesState"];
export type ModuleDeleteResult = components["schemas"]["ModulesDeleteResult"];
export const defaultModules = (): ModuleChoices => ({ proxy: false });
export interface ModulesApi {
  getModuleStates?(signal?: AbortSignal): Promise<ModuleChoices>;
  getModules?(): Promise<ModuleState[]>;
  setModuleEnabled?(id: ModuleID, enabled: boolean): Promise<void>;
  deleteModuleData?(id: ModuleID): Promise<ModuleDeleteResult>;
}
