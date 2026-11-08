// Source: docklet-next/issues/421
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original neutral password label proofs for the forced change and first-run setup.

import { screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";
import { InitialPasswordPage } from "./InitialPasswordPage";
import SetupPage from "./SetupPage";
import { restartSetup, setupStore } from "./setup-draft";

afterEach(() => { setAppLocale("de"); restartSetup(); simulationStore.reset(); });
it.each(["en-US", "de"] as const)("%s forced change is neutral for every user; first-run setup keeps administrator wording", async (locale) => {
  setAppLocale(locale); simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const demo = createDemoAdapter(); const adapter = { ...demo, kind: "http" as const, getAuthMode: async () => "local" as const, getCurrentUser: async () => ({ id: "member", username: "member", isAdmin: false, requiresPasswordChange: true }) };
  const view = renderRoutes([{ path: "/password", element: <InitialPasswordPage /> }], ["/password"], adapter);
  await screen.findByRole("heading", { name: locale === "de" ? "Neues Passwort festlegen" : "Set a new password" });
  for (const label of locale === "de" ? ["Bisheriges Passwort", "Neues Passwort", "Neues Passwort bestätigen"] : ["Current password", "New password", "Confirm new password"]) expect(screen.getByLabelText(label) !== null).toBe(true);
  expect(document.body.textContent?.includes(locale === "de" ? "Administratorpasswort" : "administrator password")).toBe(false);
  view.unmount(); restartSetup(); setupStore.set((s) => ({ ...s, reached: 2 }));
  renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup?step=auth"], adapter);
  await screen.findByLabelText(locale === "de" ? "Neues Administratorpasswort" : "New administrator password");
  expect(screen.getByLabelText(locale === "de" ? "Bisheriges Administratorpasswort" : "Current administrator password") !== null).toBe(true);
});
