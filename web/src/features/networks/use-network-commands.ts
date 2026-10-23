import { useCallback } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { AdapterOutcomeError } from "@/data/capability";
import { errorCode, type ConnectNetworkInput, type CreateNetworkInput } from "@/data/api";
import { useAdapter } from "@/data/use-adapter";
import { useAdapterText } from "@/data/use-adapter-text";

const KNOWN = ["not-found", "exists", "invalid-network", "builtin-network", "compose-managed", "in-use", "host-offline", "host-mismatch", "network-mode", "already-connected", "not-connected", "invalid-ip", "overlap"] as const;
type Known = (typeof KNOWN)[number];

/** Network commands; contract error codes become translated toasts, results refresh through change topics. */
export function useNetworkCommands() {
  const adapter = useAdapter();
  const t = useTranslations("networks.errors");
  const dt = useAdapterText();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (e) {
        if (e instanceof AdapterOutcomeError && e.outcome.kind === "error" && e.outcome.code === "network.outcome-unknown") { toast.error(t("outcome-unknown")); return undefined; }
        const code = errorCode(e);
        toast.error(code && (KNOWN as readonly string[]).includes(code) ? t(code as Known) : dt(e instanceof Error ? e.message : String(e)));
        return undefined;
      }
    },
    [t, dt],
  );
  return {
    create: (input: CreateNetworkInput) => guard(() => adapter.createNetwork(input)),
    remove: (id: string) => guard(async () => (await adapter.removeNetwork(id), true)),
    connect: (input: ConnectNetworkInput) => guard(() => adapter.connectNetwork(input)),
    disconnect: (networkId: string, containerId: string) => guard(async () => (await adapter.disconnectNetwork({ networkId, containerId }), true)),
  };
}
