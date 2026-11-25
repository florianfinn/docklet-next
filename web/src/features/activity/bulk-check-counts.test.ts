// Source: docklet-next/issues/626
// Revision: 505d1715fdf1dc1668655e9affef78b67620f7d9
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bulk check presentation and producer-shaped regression coverage.

import { createTranslator } from "use-intl";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { formatBulkCheckCounts } from "./bulk-check-counts";

const empty = { unit: "target" as const, planned: 0, plannedKnown: true, completed: 0, succeeded: 0, failed: 0, skipped: 0, updates: 0 };
it.each(["en-US", "de"] as const)("formats empty, all-failed, all-skipped and unknown plans without zero outcome parts (%s)", (locale) => {
  const t = createTranslator({ locale, messages: catalogs[locale], namespace: "activity.bulk" });
  const text = (counts: Partial<typeof empty>, running = false) => formatBulkCheckCounts({ ...empty, ...counts }, running, t);
  expect(text({})).toBe(locale === "de" ? "0 von 0 geprüft" : "0 of 0 checked");
  expect(text({ planned: 2, completed: 2, failed: 2 })).toBe(locale === "de" ? "2 fehlgeschlagen" : "2 failed");
  expect(text({ planned: 2, completed: 2, skipped: 2 })).toBe(locale === "de" ? "2 übersprungen" : "2 skipped");
  expect(text({ plannedKnown: false, completed: 1, succeeded: 1 }, true)).toBe(locale === "de" ? "1 bisher geprüft" : "1 checked so far");
  expect(text({ plannedKnown: false, completed: 1, succeeded: 1 })).toBe(locale === "de" ? "1 geprüft" : "1 checked");
  expect(text({ planned: 1, completed: 1, succeeded: 1, updates: 1 })).toBe(locale === "de" ? "1 von 1 geprüft · 1 mit Updates" : "1 of 1 checked · 1 with updates");
});
