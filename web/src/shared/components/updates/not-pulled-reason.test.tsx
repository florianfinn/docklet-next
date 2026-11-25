// Source: docklet-next/web/src/shared/components/updates/update-panel.tsx
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original regression: the stack update hover names images absent on the host from producer counts.

import { cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp } from "@/test/render";
import { StackUpdatePanel } from "./update-panel";

afterEach(cleanup);
const checked = () => document.querySelector('[data-time="checked"]')?.textContent ?? "";

it("names missing images instead of counting them as unchecked when they are the only gap", () => {
  setAppLocale("en-US");
  const current = { repository: "example/agent", tag: "2.0.0" };
  renderWithApp(<StackUpdatePanel services={[{ name: "agent", update: { state: "unknown", current, checkedAt: "2026-11-25T10:00:00Z", incomplete: true } }]} gaps={{ missing: 0, notPulled: 1 }} />);
  expect([checked().includes("1 image not on host"), checked().includes("not checked")]).toEqual([true, false]);
});

it("keeps the missing-image reason when no shown service has a check time", () => {
  setAppLocale("en-US");
  const current = { repository: "example/agent", tag: "3.0.0" };
  renderWithApp(<StackUpdatePanel services={[{ name: "agent", update: { state: "unchecked", current } }]} gaps={{ missing: 0, notPulled: 1 }} />);
  expect([checked().includes("Never checked"), checked().includes("1 image not on host")]).toEqual([true, true]);
});
