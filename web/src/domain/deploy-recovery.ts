// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original evidence-bound deployment recovery eligibility.

import type { ActionRun } from "./model";
export function hasDeployRecovery(run: ActionRun): boolean {
  return ["project_deploy", "project_redeploy"].includes(run.sourceOperation ?? "") && run.status === "failed" && !!run.restorePointId && !!run.stackId;
}
