import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";

import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { HubSettings, SettingsSection } from "@/domain/settings";

/** Saves one section through the adapter and refreshes the cached settings. Throws on failure. */
export function useSaveSettings() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useCallback(
    async <K extends SettingsSection>(section: K, value: HubSettings[K]) => {
      await adapter.saveSettings(section, value);
      await client.invalidateQueries({ queryKey: queryKeys.settings });
    },
    [adapter, client],
  );
}
