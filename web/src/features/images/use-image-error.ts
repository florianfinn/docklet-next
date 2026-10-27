// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useCallback } from "react";
import { useTranslations } from "use-intl";
import { AdapterOutcomeError } from "@/data/capability";
import { useCommandError } from "@/data/use-command-error";
import type { enImageErrors } from "./messages/errors-en";

export function useImageError() {
  const t = useTranslations("images.errors"), fallback = useCommandError();
  return useCallback((error: unknown) => {
    const code = typeof error === "string" ? error : error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : undefined;
    const key = code?.replaceAll(".", "_") as keyof typeof enImageErrors | undefined;
    return key && t.has(key) ? t(key) : fallback(error);
  }, [t, fallback]);
}
