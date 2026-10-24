// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import { AdapterOutcomeError } from "@/data/capability";
import type { CommandFailure } from "@/shared/components/command-failure";
import { openActivity } from "./activity-store";

export function useRuntimeError() {
  const t = useTranslations("activity.runtime");
  return useCallback((error: unknown, report: CommandFailure["error"] = toast.error): boolean => {
    if (!(error instanceof AdapterOutcomeError)) return false;
    const outcome = error.outcome;
    const code = outcome.kind === "conflict" || outcome.kind === "error" ? outcome.code : "unavailable";
    const key = code === "run.busy" ? "busy" : code === "run.cancelling" ? "cancellingConflict" : code === "run.request-mismatch" ? "mismatch" :
      code === "runtime.admission-unknown" ? "ambiguous" : code === "runtime.parent-unresolved" ? "parentUnresolved" :
      code === "runtime.target-removed" ? "targetRemoved" : code === "runtime.target-changed" ? "targetChanged" :
      code === "runtime.permissions-unavailable" ? "permissionsUnavailable" : code === "runtime.pre-admission" ? "preAdmission" :
      code === "runtime.release-invalid" ? "releaseInvalid" : outcome.kind === "error" && outcome.status === 401 ? "signIn" :
      outcome.kind === "error" && outcome.status === 403 ? "denied" : "requestFailed";
    const id = outcome.kind === "conflict" ? outcome.activeRunId : undefined;
    report(t(key), { action: id ? { label: t("openRun"), onClick: () => openActivity(id) } : undefined });
    return true;
  }, [t]);
}
