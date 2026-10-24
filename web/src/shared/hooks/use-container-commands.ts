import { captureCommandFailure } from "@/shared/components/command-failure";
import { isCancelledError } from "@tanstack/react-query";
import { useCallback } from "react";
import { useTranslations } from "use-intl";

import type { ContainerCommand, RecreateInput } from "@/data/adapter";
import { queueDemoFailure, type DemoFailure } from "@/runtime";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import type { ContainerOrigin } from "@/domain/model";
import { useRuntimeError } from "@/features/activity/use-runtime-error";
import { useCommandError } from "@/data/use-command-error";

const KNOWN = new Set(["recreate-not-allowed", "managed-by-unraid", "not-found"]);

/** Container commands; adapter error codes become translated command feedback. */
export function useContainerCommands() {
  const adapter = useAdapter();
  const t = useTranslations("containers.errors");
  const commandError = useCommandError();
  const runtimeError = useRuntimeError();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      const failure = captureCommandFailure(adapter);
      try {
        return await fn();
      } catch (e) {
        // The gate already reported the busy target once (see ActionToasts).
        if (isCancelledError(e) || isRunBusy(e) || runtimeError(e, failure.error)) return undefined;
        const code = e instanceof Error ? e.message : String(e);
        failure.error(KNOWN.has(code) ? t(code as "not-found") : commandError(e));
        return undefined;
      }
    },
    [t, commandError, runtimeError, adapter],
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
