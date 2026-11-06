// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue-backed bulk result reasons.

import { useTranslations } from "use-intl";
import { wireKey } from "@/data/wire-catalogue";
import { MESSAGE_CODE_KEYS } from "@/data/wire-catalogue-messages";
import { AdapterOutcomeError } from "@/data/capability";
import { useImageError } from "@/features/images/use-image-error";

export function useBulkReason() {
  const all = useTranslations(), t = useTranslations("common.bulkRun"), fallback = useImageError();
  return (code?: string, error?: unknown) => code === "session.ended" ? t("sessionEnded") : code === "bulk.up-to-date" ? t("upToDate") : code === "bulk.not-checked" ? t("notChecked") : code === "inventory.stale" ? t("stale") : code === "run.busy" || code === "run.cancelling" ? t("busy") : wireKey(MESSAGE_CODE_KEYS, code) ? all(wireKey(MESSAGE_CODE_KEYS, code)!) : fallback(error ?? new AdapterOutcomeError({ kind: "error", code: code ?? "http.error" }));
}
