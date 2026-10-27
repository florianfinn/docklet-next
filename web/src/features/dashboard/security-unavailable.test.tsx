// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource inventory contract tests.

import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { resourceFixture } from "@/data/http/resources.test-support";
import { renderWithApp } from "@/test/render";
import DashboardPage from "./DashboardPage";
import SecurityPage from "@/features/security/SecurityPage";

afterEach(() => setAppLocale("de"));
it.each(["de", "en-US"] as const)("shows unavailable security capability in dashboard and security page in %s", async (locale) => {
  setAppLocale(locale);
  const f = resourceFixture();
  const dashboard = renderWithApp(<DashboardPage />, f.adapter);
  const card = await waitFor(() => {
    const element = document.querySelector<HTMLElement>('[data-widget="security"]');
    expect(element !== null).toBe(true);
    return element!;
  });
  await waitFor(() => expect(card.textContent).toContain(locale === "de" ? "Diese Funktion ist nicht verfügbar" : "This feature is unavailable"));
  expect(card.textContent).not.toContain(locale === "de" ? "Daten konnten nicht geladen werden" : "Data could not be loaded");
  expect(within(card as HTMLElement).queryByRole("button", { name: locale === "de" ? "Erneut versuchen" : "Try again" }) === null).toBe(true);
  dashboard.unmount();
  renderWithApp(<SecurityPage />, f.adapter);
  await waitFor(() => expect(screen.getByText(locale === "de" ? "Diese Funktion ist nicht verfügbar" : "This feature is unavailable").textContent).toBeDefined());
});

it.each(["de", "en-US"] as const)("keeps another host's live findings visible when one host is unavailable in %s", async (locale) => {
  const { securityFixture } = await import("@/data/http/vulnerabilities.test-support");
  setAppLocale(locale);
  const f = securityFixture(); f.state.failureHost = "second";
  renderWithApp(<SecurityPage />, f.adapter);
  await waitFor(() => expect(screen.getByRole("button", { name: locale === "de" ? "Jetzt scannen" : "Scan now" }).hasAttribute("disabled")).toBe(false));
  await waitFor(() => expect(document.body.textContent).toContain(locale === "de" ? "Scan-Daten für second nicht verfügbar" : "Scan data unavailable for second"));
  expect(document.body.textContent).toContain("CVE-2026-0001");
  expect(document.body.textContent).not.toContain(locale === "de" ? "synthetisch" : "synthetic");
});
