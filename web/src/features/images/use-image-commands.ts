import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { errorCode, type PullImageInput } from "@/data/api";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import { queueDemoFailure } from "@/runtime";
import type { PruneMode } from "@/domain/images";
import { useAdapterText } from "@/data/use-adapter-text";

const KNOWN = ["in-use", "host-offline", "invalid-name", "not-found"] as const;

/** Image commands with translated errors; pulls and prunes report progress through action toasts. */
export function useImageCommands() {
  const adapter = useAdapter();
  const t = useTranslations("images");
  const dt = useAdapterText();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (e) {
        // The gate already reported the busy target once (see ActionToasts).
        if (isRunBusy(e)) return undefined;
        const code = errorCode(e);
        toast.error(code && (KNOWN as readonly string[]).includes(code) ? t(`errors.${code as (typeof KNOWN)[number]}`) : dt(e instanceof Error ? e.message : String(e)));
        return undefined;
      }
    },
    [t, dt],
  );
  return {
    pull: (input: PullImageInput, fail = false) => guard(() => {
      if (fail) queueDemoFailure(adapter, { operation: "pullImage" });
      return adapter.pullImage(input);
    }),
    remove: (id: string, name: string) =>
      guard(async () => {
        await adapter.removeImage(id);
        toast.success(t("toast.removed", { name }));
        return true;
      }),
    prune: (hostIds: string[], mode: PruneMode) =>
      guard(async () => {
        const id = await adapter.pruneImages({ hostIds, mode });
        if (!id) toast(t("toast.nothing"));
        return id;
      }),
  };
}
