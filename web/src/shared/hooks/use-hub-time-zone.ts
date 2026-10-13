import { useSettings } from "@/data/queries";
import { DEFAULT_TIME_ZONE } from "@/domain/schedule";

/** Time zone of all schedules: the hub setting, Europe/Berlin until settings are loaded. */
export function useHubTimeZone(): string {
  const settings = useSettings();
  return settings.data?.general.timeZone || DEFAULT_TIME_ZONE;
}
