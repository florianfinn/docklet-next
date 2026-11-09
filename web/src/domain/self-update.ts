// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original browser self-update model and frozen DTO validation; M21 R1 version comparison.

import type { components } from "@/api/generated/schema";
import { SELF_UPDATE_CODE_KEYS, SELF_UPDATE_STATE_KEYS, SELF_UPDATE_PHASE_KEYS, SELF_UPDATE_OUTCOME_KEYS } from "@/data/wire-catalogue-self-update";
import { HOST_CODE_KEYS, IMAGE_ERROR_KEYS, ACCESS_CODE_KEYS } from "@/data/wire-catalogue-errors";
import { wireKey } from "@/data/wire-catalogue";
import type { ResourceRef } from "@/data/capability";

type Schema = components["schemas"];
export type SelfUpdateCheck = Schema["SystemSelfUpdateCheck"];
export type SelfUpdateResponse = Schema["SystemSelfUpdateResponse"];
export type SelfUpdateInput = Pick<Schema["SystemSelfUpdateEnvelope"], "targetVersion">;
export type SelfUpdateState = Omit<Schema["SystemSelfUpdateEligibility"], "manual"> & { manual: string[] };
export const selfUpdateRef = (host: string): ResourceRef => host === "0" ? { kind: "hub", id: "0" } : { kind: "host", id: host };
export const selfUpdateState = (check?: SelfUpdateCheck): SelfUpdateState => check ? { ...check.eligibility, manual: check.eligibility.manual ?? [] } : { state: "checking", manual: [] };
// /api/app-version reports "v1.2.3" while the release feed target is "1.2.3".
export const sameProductVersion = (a?: string, b?: string): boolean => !!a && !!b && a.replace(/^v/, "") === b.replace(/^v/, "");
export const canSelfUpdate = (state: SelfUpdateState): boolean => state.state === "available" && !!state.candidate?.version;
export const selfUpdateCodeKey = (code: string | undefined) => wireKey(SELF_UPDATE_CODE_KEYS, code) ?? wireKey(HOST_CODE_KEYS, code) ?? wireKey(IMAGE_ERROR_KEYS, code) ?? wireKey(ACCESS_CODE_KEYS, code);

export function validSelfUpdateCheck(value: SelfUpdateCheck): boolean {
  const e = value?.eligibility, op = value?.operation;
  return typeof value?.version === "string" && !!e && !!wireKey(SELF_UPDATE_STATE_KEYS, e.state)
    && (e.manual === null || Array.isArray(e.manual) && e.manual.every(s => typeof s === "string"))
    && (!e.code || !!selfUpdateCodeKey(e.code))
    && (!e.candidate || typeof e.candidate.version === "string" && !!e.candidate.version)
    && (e.state !== "available" || !!e.candidate?.version)
    && (!op || typeof op.opId === "string" && !!op.opId && typeof op.targetVersion === "string"
      && typeof op.helperRunning === "boolean" && !!wireKey(SELF_UPDATE_PHASE_KEYS, op.phase)
      && (!op.outcome || !!wireKey(SELF_UPDATE_OUTCOME_KEYS, op.outcome)));
}
