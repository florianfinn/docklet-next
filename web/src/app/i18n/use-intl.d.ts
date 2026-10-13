import type { AppLocale } from "./locale-store";
import type { Messages } from "./catalog";

declare module "use-intl" {
  interface AppConfig {
    Messages: Messages;
    Locale: AppLocale;
  }
}
