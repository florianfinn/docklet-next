import { useConnectedSettings, useSettings } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { DEFAULT_TIME_ZONE } from "@/domain/schedule";

/** The process TZ drives connected schedules and display; demo data carries its own zone. */
export function useHubTimeZone(): string {
  const connected = useAdapter().kind === "http";
  const settings = useSettings(!connected);
  const hub = useConnectedSettings(connected);
  return (connected ? hub.data?.general.timeZone : settings.data?.general.timeZone) || (connected ? "UTC" : DEFAULT_TIME_ZONE);
}
