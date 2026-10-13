import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { ContainerCommand, FailAt, RecreateInput } from "@/data/adapter";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import type { ContainerOrigin } from "@/domain/model";
import { useDemoText } from "@/shared/hooks/use-demo-text";

const KNOWN = new Set(["recreate-not-allowed", "managed-by-unraid", "not-found"]);

/** Container commands; adapter error codes become translated toasts. */
export function useContainerCommands() {
  const adapter = useAdapter();
  const t = useTranslations("containers.errors");
  const dt = useDemoText();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (e) {
        // The gate already reported the busy target once (see ActionToasts).
        if (isRunBusy(e)) return undefined;
        const code = e instanceof Error ? e.message : String(e);
        toast.error(KNOWN.has(code) ? t(code as "not-found") : dt(code));
        return undefined;
      }
    },
    [t, dt],
  );
  return {
    run: (id: string, command: ContainerCommand) => guard(() => adapter.runContainerCommand(id, command)),
    recreate: (id: string, input: RecreateInput, failAt?: FailAt) => guard(() => adapter.recreateContainer(id, input, failAt)),
    update: (id: string) => guard(() => adapter.installContainerUpdate(id)),
    check: (id: string) => guard(() => adapter.checkUpdates({ containerId: id })),
    setOrigin: (id: string, origin: ContainerOrigin) => guard(() => adapter.setContainerOrigin(id, origin)),
  };
}
