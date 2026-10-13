import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { errorCode, type RegisterHostInput } from "@/data/api";
import { useActionRuns } from "@/data/live";
import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import { queueDemoFailure } from "@/runtime";
import type { ActionKind } from "@/domain/model";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { useDemoText } from "@/shared/hooks/use-demo-text";
import { preferencesStore } from "@/shared/preferences/preferences-store";

/** Host commands with translated errors; progress of connections is shown by action toasts. */
export function useHostCommands() {
  const adapter = useAdapter();
  const t = useTranslations("hosts.errors");
  const dt = useDemoText();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (e) {
        // The gate already reported the busy target once (see ActionToasts).
        if (isRunBusy(e)) return undefined;
        const code = errorCode(e);
        toast.error(code === "exists" || code === "not-found" || code === "host-in-use" ? t(code) : dt(e instanceof Error ? e.message : String(e)));
        return undefined;
      }
    },
    [t, dt],
  );
  return {
    register: (input: RegisterHostInput) => guard(() => adapter.registerHost(input)),
    connect: (hostId: string, fail?: boolean) => guard(() => {
      if (fail) queueDemoFailure(adapter, { operation: "connectHost" });
      return adapter.connectHost(hostId);
    }),
    reconnect: (hostId: string) => guard(() => adapter.reconnectHost(hostId)),
    restartAgent: (hostId: string) => guard(() => adapter.restartAgent(hostId)),
    remove: (hostId: string) =>
      guard(async () => {
        await adapter.removeHost(hostId);
        // A filter that only listed the removed host would hide everything.
        preferencesStore.set((p) => ({ ...p, hostSelection: p.hostSelection.filter((id) => id !== hostId) }));
        return true;
      }),
    token: (name: string) => guard(() => adapter.issueAgentToken(name)),
  };
}

export type HostActionKind = Extract<ActionKind, "reconnect" | "onboard" | "agent-restart">;

/**
 * One host action button: busy while its own call runs and while any run of
 * the same kind for this host is still running, so the host card and the
 * settings dialog agree no matter where the action was started.
 */
export function useHostAction(hostId: string, kind: HostActionKind, fn: () => Promise<string | undefined>) {
  const local = useBusyAction(fn);
  const running = useActionRuns().some((r) => r.hostId === hostId && r.kind === kind && r.status === "running");
  return { busy: local.busy || running, run: local.run };
}
