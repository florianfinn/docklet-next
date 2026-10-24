import { captureCommandFailure } from "@/shared/components/command-failure";
import { isCancelledError, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";

import type { StackCommand } from "@/data/adapter";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import { queryKeys } from "@/data/query-keys";
import { queueDemoFailure, type DemoFailure } from "@/runtime";
import type { AutoUpdatePolicy, Stack, StackDetail } from "@/domain/model";
import { useRuntimeError } from "@/features/activity/use-runtime-error";
import { useCommandError } from "@/data/use-command-error";

/** Stack commands; progress is reported by action toasts, errors here. */
export function useStackCommands() {
  const adapter = useAdapter();
  const client = useQueryClient();
  const commandError = useCommandError();
  const runtimeError = useRuntimeError();

  const guard = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      const failure = captureCommandFailure(adapter);
    try {
      return await fn();
    } catch (e) {
      // The gate already reported the busy target once (see ActionToasts).
      if (isCancelledError(e) || isRunBusy(e) || runtimeError(e, failure.error)) return undefined;
      failure.error(commandError(e));
      return undefined;
    }
  }, [commandError, runtimeError, adapter]);

  /**
   * Optimistic write of a small stack setting: the cached stack and list show
   * it at once, so a toggle answers on click instead of after the save and a
   * full simulated refetch (650 ms ±25 %, 2.4 s on "long" latency). A failed
   * save refetches the stored value.
   */
  const optimistic = useCallback(
    async (stackId: string, patch: (s: Stack) => Stack, save: () => Promise<void>) => {
      await client.cancelQueries({ queryKey: queryKeys.stack(stackId) });
      await client.cancelQueries({ queryKey: queryKeys.stacks });
      client.setQueryData<StackDetail>(queryKeys.stack(stackId), (old) => old && { ...old, ...patch(old) });
      client.setQueryData<Stack[]>(queryKeys.stacks, (old) => old?.map((s) => (s.id === stackId ? patch(s) : s)));
      try {
        await save();
      } catch (e) {
        await client.invalidateQueries({ queryKey: queryKeys.stack(stackId) });
        await client.invalidateQueries({ queryKey: queryKeys.stacks });
        throw e;
      }
    },
    [client],
  );

  return {
    run: (stackId: string, command: StackCommand, failAt?: DemoFailure["at"]) => guard(() => {
      if (failAt) queueDemoFailure(adapter, { operation: "runStackCommand", at: failAt });
      return adapter.runStackCommand(stackId, command);
    }),
    installUpdates: (stackId: string, services?: string[]) => guard(() => adapter.installStackUpdates(stackId, services)),
    checkUpdates: (stackId: string) => guard(() => adapter.checkUpdates({ stackId })),
    restartService: (stackId: string, service: string) => guard(() => adapter.restartService(stackId, service)),
    restore: (stackId: string, restorePointId: string) => guard(() => adapter.restoreRestorePoint(stackId, restorePointId)),
    setLabels: (stackId: string, labels: string[]) =>
      guard(async () => {
        await optimistic(stackId, (s) => ({ ...s, labels }), () => adapter.setStackLabels(stackId, labels));
        await client.invalidateQueries({ queryKey: queryKeys.stack(stackId) });
      }),
    setAutoUpdate: (stackId: string, policy: AutoUpdatePolicy) => guard(() => optimistic(stackId, (s) => ({ ...s, autoUpdate: policy }), () => adapter.setAutoUpdate(stackId, policy))),
  };
}
