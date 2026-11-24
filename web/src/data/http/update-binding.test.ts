// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original typed eligibility and legacy refusal decoding proofs.

import { expect, it } from "vitest";
import { imageBinding, updateEligibility } from "./update-binding";
import { IMAGE_BINDING_KEYS } from "../wire-catalogue-update";
it("preserves binding detail at the typed eligibility boundary", () => {
  const binding = { scope: "service", service: "app", reason: "variable-reference", variable: "SERVICE_IMAGE" };
  expect(updateEligibility({ services: [{ service: "app", eligible: false, binding }] }).services[0]?.binding).toEqual(binding);
});
it("validates every declared reason and rejects inconsistent scope and variable fields", () => {
  for (const reason of Object.keys(IMAGE_BINDING_KEYS)) expect(imageBinding({ scope: "project", reason }).reason).toBe(reason);
  for (const value of [{ scope: "service", reason: "build-service" }, { scope: "project", service: "app", reason: "include-extends" }, { scope: "project", reason: "other" }, { scope: "service", service: "app", reason: "build-service", variable: "IMAGE" }, { scope: "service", service: "app", reason: "variable-reference", variable: "${IMAGE:-secret}" }]) expect(() => imageBinding(value)).toThrow();
});
it("rejects missing bindings and duplicate service identities", () => {
  expect(() => updateEligibility({ services: [{ service: "app", eligible: false }] })).toThrow();
  expect(() => updateEligibility({ services: [{ service: "app", eligible: true }, { service: "app", eligible: true }] })).toThrow();
});

it("preserves a v23 refusal binding and accepts a v22 refusal without one", async () => {
 const { runtimeResponseError } = await import("./runtime-mapping");
 const binding = { scope: "service", service: "app", reason: "variable-reference", variable: "SERVICE_IMAGE" };
 for (const detail of [undefined,binding]) {
  const error = runtimeResponseError({success:false,error:{code:"update.image-binding-unsupported",message:"producer fallback",...(detail?{binding:detail}:{})}},409);
  expect(error.outcome.kind).toBe("error");
  if (error.outcome.kind === "error") expect(error.outcome.binding).toEqual(detail);
 }
});
