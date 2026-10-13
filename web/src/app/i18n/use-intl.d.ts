import type { AppLocale } from "./locale-store";
import type { messages } from "./messages";

declare module "use-intl" {
  interface AppConfig {
    Messages: typeof messages;
    Locale: AppLocale;
  }
}
