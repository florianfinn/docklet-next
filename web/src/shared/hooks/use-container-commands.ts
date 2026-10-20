import { isCancelledError } from "@tanstack/react-query";
import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { ContainerCommand, RecreateInput } from "@/data/adapter";
import { queueDemoFailure, type DemoFailure } from "@/runtime";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import type { ContainerOrigin } from "@/domain/model";
import { useRuntimeError } from "@/features/activity/use-runtime-error";
import { useAdapterText } from "@/data/use-adapter-text";

const KNOWN = new Set(["recreate-not-allowed", "managed-by-unraid", "not-found"]);

/** Container commands; adapter error codes become translated toasts. */
export function useContainerCommands() {
  const adapter = useAdapter();
  const t = useTranslations("containers.errors");
  const dt = useAdapterText();
  const runtimeError = useRuntimeError();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (e) {
        // The gate already reported the busy target once (see ActionToasts).
        if (isCancelledError(e) || isRunBusy(e) || runtimeError(e)) return undefined;
        const code = e instanceof Error ? e.message : String(e);
        toast.error(KNOWN.has(code) ? t(code as "not-found") : dt(code));
        return undefined;
      }
    },
    [t, dt, runtimeError],
  );
  return {
    run: (id: string, command: ContainerCommand) => guard(() => adapter.runContainerCommand(id, command)),
    recreate: (id: string, input: RecreateInput, failAt?: DemoFailure["at"]) => guard(() => {
      if (failAt) queueDemoFailure(adapter, { operation: "recreateContainer", at: failAt });
      return adapter.recreateContainer(id, input);
    }),
    update: (id: string) => guard(() => adapter.installContainerUpdate(id)),
    check: (id: string) => guard(() => adapter.checkUpdates({ containerId: id })),
    setOrigin: (id: string, origin: ContainerOrigin) => guard(() => adapter.setContainerOrigin(id, origin)),
  };
}
