import { captureCommandFailure } from "@/shared/components/command-failure";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useTranslations } from "use-intl";

import { errorCode, type CreateVolumeInput } from "@/data/api";
import { isRunBusy } from "@/data/run-gate";
import { AdapterOutcomeError } from "@/data/capability";
import { useAdapter } from "@/data/use-adapter";
import type { BackupPolicy } from "@/domain/volumes";
import { wireKey } from "@/data/wire-catalogue";
import { VOLUME_COMMAND_KEYS } from "@/data/wire-catalogue-backup";
import { useCommandError } from "@/data/use-command-error";

type Known = "not-found" | "exists" | "invalid-name" | "in-use" | "host-offline" | "bind-not-deletable" | "not-editable" | "no-policy" | "target-missing" | "invalid-plan";
const KNOWN = new Set<string>(["not-found", "exists", "invalid-name", "in-use", "host-offline", "bind-not-deletable", "not-editable", "no-policy", "target-missing", "invalid-plan"]);

/** Volume, file and backup commands; contract error codes become translated command feedback. */
export function useVolumeCommands() {
  const adapter = useAdapter();
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const t = useTranslations("volumes.errors"), all = useTranslations();
  const commandError = useCommandError();
  const guard = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      const failure = captureCommandFailure(adapter);
      try {
        const result = await fn();
        return mounted.current ? result : undefined;
      } catch (e) {
        if (!mounted.current && !failure.closed()) return undefined;
        // The gate already reported the busy target once (see ActionToasts).
        if (isRunBusy(e)) return undefined;
        if (e instanceof AdapterOutcomeError && e.outcome.kind === "error") {
          const code = e.outcome.code;
          if (code === "volume.outcome-unknown" || code === "volume.helper-incompatible" || code === "volume.helper-unavailable") {
            failure.error(code === "volume.outcome-unknown" ? t("outcome-unknown") : all(VOLUME_COMMAND_KEYS[code]));
            return undefined;
          }
        }
        const code = errorCode(e);
        const key = wireKey(VOLUME_COMMAND_KEYS, code === "exists" ? "volume.exists" : code);
        failure.error(key ? all(key) : code && KNOWN.has(code) ? t(code as Known) : commandError(e));
        return undefined;
      }
    },
    [t, commandError, adapter, all],
  );
  return useMemo(
    () => ({
      create: (input: CreateVolumeInput) => guard(() => adapter.createVolume(input)),
      renameVolume: (id: string, name: string) => guard(() => adapter.renameVolume(id, name)),
      remove: (id: string) => guard(async () => (await adapter.deleteVolume(id), true)),
      prune: (input: Parameters<typeof adapter.pruneVolumes>[0]) => guard(() => adapter.pruneVolumes(input)),
      read: (id: string, path: string) => guard(() => adapter.readFile(id, path)),
      write: (id: string, path: string, content: string) => guard(async () => (await adapter.writeFile(id, path, content), true)),
      createEntry: (id: string, dir: string, name: string, type: "dir" | "file") => guard(async () => (await adapter.createEntry(id, dir, name, type), true)),
      rename: (id: string, path: string, name: string) => guard(async () => (await adapter.renameEntry(id, path, name), true)),
      removeEntry: (id: string, path: string) => guard(async () => (await adapter.deleteEntry(id, path), true)),
      upload: (id: string, dir: string, file: { name: string; size: number; content?: string }) => guard(async () => (await adapter.uploadFile(id, dir, file), true)),
      setPolicy: (id: string, policy: BackupPolicy | null) => guard(async () => (await adapter.setBackupPolicy(id, policy), true)),
      backup: (id: string) => guard(() => adapter.runBackup(id)),
      restore: (id: string, runId: string, mode: "replace" | "new", newVolumeName?: string) =>
        guard(() => adapter.restoreBackup(id, runId, mode, newVolumeName)),
    }),
    [adapter, guard],
  );
}
