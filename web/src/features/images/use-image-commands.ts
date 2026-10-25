import { openActivity } from "@/features/activity/activity-store";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { errorCode, type PullImageInput } from "@/data/api";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import { queueDemoFailure } from "@/runtime";
import type { PruneMode } from "@/domain/images";
import { useImageError } from "./use-image-error";

const KNOWN = ["in-use", "host-offline", "invalid-name", "not-found"] as const;

/** Image commands preserve typed inline failures and optional pull activity links. */
export function useImageCommands() {
  const adapter = useAdapter();
  const t = useTranslations("images");
  const commandError = useImageError();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      const failure = captureCommandFailure(adapter);
      try {
        return await fn();
      } catch (e) {
        // The gate already reported the busy target once (see ActionToasts).
        if (isRunBusy(e)) return undefined;
        const code = errorCode(e);
        const actionId = e && typeof e === "object" && "actionId" in e && typeof e.actionId === "string" ? e.actionId : undefined;
        failure.error(code && (KNOWN as readonly string[]).includes(code) ? t(`errors.${code as (typeof KNOWN)[number]}`) : commandError(e), actionId ? { action: { label: t("followActivity"), onClick: () => openActivity(actionId) } } : undefined);
        return undefined;
      }
    },
    [t, commandError, adapter],
  );
  return {
    pull: (input: PullImageInput, fail = false) => guard(() => {
      if (fail) queueDemoFailure(adapter, { operation: "pullImage" });
      return adapter.pullImage(input);
    }),
    remove: (id: string, name: string, force = false) =>
      guard(async () => {
        const failureCurrent = captureCommandFailure(adapter).current;
        await adapter.removeImage(id, force);
        if (!failureCurrent()) return false;
        toast.success(t("toast.removed", { name }));
        return true;
      }),
    prune: (hostIds: string[], mode: PruneMode) =>
      guard(async () => {
        return adapter.pruneImages({ hostIds, mode });
      }),
  };
}
