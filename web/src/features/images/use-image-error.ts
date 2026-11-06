// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230. M19: total contract-code catalogue maps.

import { useCallback } from "react";
import { useTranslations } from "use-intl";
import { AdapterOutcomeError } from "@/data/capability";
import { useCommandError } from "@/data/use-command-error";
import { wireKey } from "@/data/wire-catalogue";
import { IMAGE_ERROR_KEYS } from "@/data/wire-catalogue-errors";
import type { enImageErrors } from "./messages/errors-en";

export function useImageError() {
  const all = useTranslations(), t = useTranslations("images.errors"), fallback = useCommandError();
  return useCallback((error: unknown) => {
    const code = typeof error === "string" ? error : error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : undefined;
    const guardedKey = wireKey(IMAGE_ERROR_KEYS, code);
    if (guardedKey) return all(guardedKey);
    const key = code?.replaceAll(".", "_") as keyof typeof enImageErrors | undefined;
    return key && t.has(key) ? t(key) : fallback(error);
  }, [t, fallback, all]);
}
