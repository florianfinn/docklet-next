// Source: docklet-next/issues/595
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original visible update check with request-owned feedback.

import { ScanSearch } from "lucide-react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import { Button } from "@/shared/ui/button";
import { useUpdateAction } from "./use-update-action";

export interface CheckFeedback { hasUpdate: boolean; version?: string }

export function CheckUpdatesButton({ owner, disabled, title, check }: { owner: string; disabled?: boolean; title?: string; check: () => Promise<CheckFeedback> }) {
  const t = useTranslations("containers.updateCheck"), action = useUpdateAction(owner, t("unfinished"));
  return <div className="flex min-w-0 flex-col gap-1">
    <Button size="sm" variant="outline" disabled={disabled || action.busy} title={title} aria-busy={action.busy} onClick={() => void action.run(check, (result) => toast.success(result.hasUpdate ? t("available", { version: result.version || "—" }) : t("current")))}>
      <ScanSearch data-icon="inline-start" className={action.busy ? "animate-pulse" : undefined} />{t(action.busy ? "checking" : "check")}
    </Button>
    {action.error && <p role="alert" className="max-w-sm text-xs">{action.error}</p>}
  </div>;
}
