// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original optional-module HTTP reads and management writes.

import { defaultModules, MODULES, type ModuleChoices, type ModulesApi, type ModuleState, type ModuleDeleteResult } from "@/domain/modules";
import { AdapterOutcomeError } from "../capability";
import { moduleProblem } from "../module-error";
import { settingsProblem } from "../settings-error";
import type { CsrfClient } from "./csrf-client";

export async function moduleResponse(response: Response): Promise<unknown> {
  const body: unknown = await response.json().catch(() => undefined);
  if (!response.ok) throw moduleProblem(body, response.status) ?? settingsProblem(body, response.status);
  return body;
}
const invalid = () => new AdapterOutcomeError({ kind: "error", code: "module.invalid-response" });
function data(value: unknown): unknown {
  if (!value || typeof value !== "object" || !("success" in value) || value.success !== true || !("data" in value)) throw invalid();
  return value.data;
}
export function createModulesApi(baseUrl: string, client: CsrfClient): ModulesApi {
  return {
    async getModuleStates(signal) {
      const body = await moduleResponse(await client.fetch(`${baseUrl}/environments/0/settings/public`, { signal, headers: { Accept: "application/json" } }));
      if (!Array.isArray(body)) throw invalid();
      const states = defaultModules();
      for (const module of MODULES) {
        const raw = body.find((v: unknown) => v && typeof v === "object" && "key" in v && v.key === module.settingKey)?.value;
        if (raw !== undefined && raw !== "true" && raw !== "false") throw invalid();
        states[module.id] = raw === "true";
      }
      return states;
    },
    async getModules() {
      const rows = data(await moduleResponse(await client.fetch(`${baseUrl}/modules`, { headers: { Accept: "application/json" } })));
      if (!Array.isArray(rows) || rows.length !== MODULES.length || !MODULES.every((m) => rows.some((r) => r?.id === m.id && typeof r.enabled === "boolean" && r.kind === "module"))) throw invalid();
      return rows as ModuleState[];
    },
    async setModuleEnabled(id, enabled) {
      const definition = MODULES.find((m) => m.id === id);
      if (!definition) throw invalid();
      await moduleResponse(await client.fetch(`${baseUrl}/environments/0/settings`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ [definition.settingKey]: String(enabled) }) }));
    },
    async deleteModuleData(id) {
      const result = data(await moduleResponse(await client.fetch(`${baseUrl}/modules/${encodeURIComponent(id)}/data`, { method: "DELETE" })));
      if (!result || typeof result !== "object" || !("pendingCleanup" in result) || !Array.isArray(result.pendingCleanup) || !result.pendingCleanup.every((v) => v && typeof v.id === "string" && typeof v.name === "string" && typeof v.reason === "string")) throw invalid();
      return result as ModuleDeleteResult;
    },
  };
}

export function moduleSettingValues(choices: ModuleChoices | undefined): Record<string, string> {
  return Object.fromEntries(MODULES.map((module) => [module.settingKey, String(choices?.[module.id] ?? false)]));
}
