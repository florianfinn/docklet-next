// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original typed binding and read-only eligibility decoders.

import type { components } from "@/api/generated/schema";
import { IMAGE_BINDING_KEYS } from "../wire-catalogue-update";
import { bool, choice, id, invalidUpdate, record } from "./update-validation";
export type ImageBinding = components["schemas"]["ImageBinding"];
import type { UpdateEligibility } from "@/domain/stack-updates";
export type { UpdateEligibility } from "@/domain/stack-updates";
export function imageBinding(value: unknown): ImageBinding {
  const o = record(value), scope = choice(o.scope, ["project", "service"]), reason = choice(o.reason, Object.keys(IMAGE_BINDING_KEYS) as ImageBinding["reason"][]);
  if (scope === "project" && o.service !== undefined || scope === "service" && (typeof o.service !== "string" || !o.service.trim() || o.service.length > 128)) throw invalidUpdate();
  const variable = o.variable === undefined ? undefined : id(o.variable);
  if (variable !== undefined && (reason !== "variable-reference" || !/^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(variable))) throw invalidUpdate();
  return { scope, reason, ...(scope === "service" ? { service: id(o.service) } : {}), ...(variable === undefined ? {} : { variable }) };
}
export function updateEligibility(value: unknown): UpdateEligibility {
  const o = record(value);
  if (!Array.isArray(o.services) || o.services.length > 256) throw invalidUpdate();
  const names = new Set<string>();
  const services = o.services.map((v) => {
    const row = record(v), service = id(row.service), eligible = bool(row.eligible);
    if (service.length > 128 || names.has(service)) throw invalidUpdate(); names.add(service);
    const binding = row.binding === undefined ? undefined : imageBinding(row.binding);
    if (eligible && binding || !eligible && !binding || binding?.scope === "service" && binding.service !== service) throw invalidUpdate();
    return { service, eligible, ...(binding ? { binding } : {}) };
  });
  const project = o.project === undefined ? undefined : imageBinding(o.project);
  if (project && project.scope !== "project") throw invalidUpdate();
  return { services, ...(project ? { project } : {}) };
}
