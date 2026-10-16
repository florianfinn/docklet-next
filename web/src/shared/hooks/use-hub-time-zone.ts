import { useConnectedSettings, useSettings } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { DEFAULT_TIME_ZONE } from "@/domain/schedule";

/** Time zone of all schedules: the hub setting, Europe/Berlin until settings are loaded. */
export function useHubTimeZone(): string {
  const connected = useAdapter().kind === "http";
  const settings = useSettings(!connected);
  const hub = useConnectedSettings(connected);
  return (connected ? hub.data?.general.timeZone : settings.data?.general.timeZone) || DEFAULT_TIME_ZONE;
}
