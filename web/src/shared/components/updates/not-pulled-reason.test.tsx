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

it("shows producer counts for missing and unchecked images in the stack hover", () => {
  setAppLocale("en-US");
  const current = { repository: "example/agent", tag: "2.0.0" };
  const services = [{ name: "agent", update: { state: "unknown" as const, current, checkedAt: "2026-11-25T10:00:00Z", incomplete: true, notPulled: true } }];
  renderWithApp(<StackUpdatePanel services={services} gaps={{ missing: 1, notPulled: 1 }} />);
  const checked = document.querySelector('[data-time="checked"]')?.textContent ?? "";
  expect([checked.includes("1 image not on host"), checked.includes("1 image not checked")]).toEqual([true, true]);
});
