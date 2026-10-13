import type { AppLocale } from "./locale-store";
import { messages } from "./messages";
import { enMessages } from "./messages-en";

export type Messages = typeof enMessages;

export const catalogs: Record<AppLocale, Messages> = { de: messages, "en-US": enMessages };
