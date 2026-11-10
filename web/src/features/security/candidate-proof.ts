// Source: docklet-next/issues/438
// Revision: cc93f43359147e8e128b2ac00bcd410c055eb423
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original candidate proof consumer and verified update integration. M21 R1-S12W N2: clock skew tolerance.

import type { CandidateResult } from "@/data/vulnerability-candidates";

export function currentProof(candidate: CandidateResult | undefined, now = Date.now()) {
  if (!candidate || candidate.state !== "completed" || !candidate.proof || !candidate.identity) return false;
  const completed = Date.parse(candidate.proof.completedAt);
  // A browser clock slightly behind the hub still shows a fresh proof.
  return Number.isFinite(completed) && completed <= now + 300000 && now - completed < 86400000;
}
