import { captureCommandFailure } from "@/shared/components/command-failure";
import { useCallback, useLayoutEffect, useRef } from "react";
import { isCancelledError } from "@tanstack/react-query";
import { identityLifetime } from "@/data/identity-lifetime";
import { useTranslations } from "use-intl";

import { AdapterOutcomeError } from "@/data/capability";
import { errorCode, type ConnectNetworkInput, type CreateNetworkInput } from "@/data/api";
import { useAdapter } from "@/data/use-adapter";
import { NETWORK_CODE_KEYS } from "@/data/wire-catalogue-errors";
import { wireKey } from "@/data/wire-catalogue";
import { useCommandError } from "@/data/use-command-error";

const KNOWN = ["not-found", "exists", "invalid-network", "builtin-network", "compose-managed", "in-use", "host-offline", "host-mismatch", "network-mode", "already-connected", "not-connected", "invalid-ip", "overlap"] as const;
type Known = (typeof KNOWN)[number];

/** Network commands; contract error codes become translated command feedback, results refresh through change topics. */
export function useNetworkCommands(owner?: string, active = true) {
  const adapter = useAdapter();
  const generation = useRef({ version: 0 });
  useLayoutEffect(() => {
    const scope = generation.current;
    scope.version++;
    return () => { scope.version++; };
  }, [adapter, owner, active]);
  const t = useTranslations("networks.errors"), all = useTranslations();
  const commandError = useCommandError();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      const failure = captureCommandFailure(adapter);
      const started = generation.current.version;
      const lifetime = identityLifetime(adapter);
      const version = lifetime.version();
      const current = () => active && generation.current.version === started && lifetime.active() && lifetime.version() === version;
      try {
        const result = await fn();
        return current() ? result : undefined;
      } catch (e) {
        if ((!current() && !failure.closed()) || isCancelledError(e)) return undefined;
        if (e instanceof AdapterOutcomeError && e.outcome.kind === "error" && e.outcome.code === "network.outcome-unknown") { failure.error(t("outcome-unknown")); return undefined; }
        const code = errorCode(e);
        const key = wireKey(NETWORK_CODE_KEYS, code?.startsWith("network.") ? code : `network.${code}`);
        failure.error(key ? all(key) : code && (KNOWN as readonly string[]).includes(code) ? t(code as Known) : commandError(e));
        return undefined;
      }
    },
    [t, commandError, adapter, active, all],
  );
  return {
    create: (input: CreateNetworkInput) => guard(() => adapter.createNetwork(input)),
    remove: (id: string) => guard(async () => (await adapter.removeNetwork(id), true)),
    connect: (input: ConnectNetworkInput) => guard(() => adapter.connectNetwork(input)),
    disconnect: (networkId: string, containerId: string) => guard(async () => (await adapter.disconnectNetwork({ networkId, containerId }), true)),
  };
}
