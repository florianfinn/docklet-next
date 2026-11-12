// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original hub zone field guards: saved-state reset label and the typed invalid-zone refusal.

import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createSettingsCore } from "@/data/http/settings-core";
import { useCommandError } from "@/data/use-command-error";
import type { GeneralSettings } from "@/domain/settings";
import { renderWithApp } from "@/test/render";
import { GeneralFields } from "./GeneralSection";

const general: GeneralSettings = { hubName: "Hub", hubAddress: "https://hub.example.com", domainUrl: "", linkBase: "auto", timeZone: "UTC", configuredTimeZone: "" };

function Fields({ saved }: { saved: GeneralSettings }) {
  const [draft, setDraft] = useState(saved);
  return <GeneralFields value={draft} saved={saved} onChange={(p) => setDraft((d) => ({ ...d, ...p }))} showErrors={false} />;
}

it.each(["en-US", "de"] as const)("S2: with a saved override the reset label never names the override as TZ, before or after reset, in %s", async (locale) => {
  setAppLocale(locale);
  // Process TZ=UTC; the stored override makes Europe/Berlin effective.
  renderWithApp(<Fields saved={{ ...general, timeZone: "Europe/Berlin", configuredTimeZone: "Europe/Berlin" }} />);
  const reset = await screen.findByRole("button", { name: /TZ/ });
  expect(reset.textContent?.includes(catalogs[locale].settings.general.systemTZ)).toBe(true);
  await userEvent.click(reset);
  const after = screen.getByRole("button", { name: /TZ/ });
  expect(after.textContent?.includes("Europe/Berlin")).toBe(false);
  expect(screen.getByRole("combobox").getAttribute("placeholder") === "Europe/Berlin").toBe(false);
});

it.each(["en-US", "de"] as const)("S2: without a saved override a typed draft keeps the TZ value in the reset label in %s", async (locale) => {
  setAppLocale(locale);
  renderWithApp(<Fields saved={general} />);
  await userEvent.type(screen.getByRole("combobox"), "Europe/Berlin");
  expect(screen.getByRole("button", { name: /TZ/ }).textContent?.includes("(UTC)")).toBe(true);
});

function Refusal({ error }: { error: unknown }) {
  return <p>{useCommandError()(error)}</p>;
}

it.each(["en-US", "de"] as const)("SR7: a refused zone save shows the invalid-zone text in %s", async (locale) => {
  setAppLocale(locale);
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ code: "settings.invalid-time-zone", status: 400 }), { status: 400, headers: { "Content-Type": "application/problem+json" } }));
  const api = createSettingsCore("/api", { fetch: fetcher as typeof fetch, token: async () => "example" });
  const error = await api.saveSettings("general", { ...general, configuredTimeZone: "Mars/Base" }).then(() => undefined, (cause: unknown) => cause);
  expect(JSON.parse(String((fetcher.mock.calls[0] as unknown as [string, RequestInit])[1].body)).timeZone).toBe("Mars/Base");
  renderWithApp(<Refusal error={error} />);
  expect((await screen.findByText(catalogs[locale].settings.codes.settings["invalid-time-zone"])) !== null).toBe(true);
});

it.each(["en-US", "de"] as const)("#410: with a saved override the reset label names the process TZ the hub reports in %s", async (locale) => {
  setAppLocale(locale);
  renderWithApp(<Fields saved={{ ...general, timeZone: "Europe/Berlin", configuredTimeZone: "Europe/Berlin", defaultTimeZone: "Asia/Tokyo" }} />);
  const reset = await screen.findByRole("button", { name: /TZ/ });
  expect(reset.textContent?.includes("(Asia/Tokyo)")).toBe(true);
  expect(reset.textContent?.includes(catalogs[locale].settings.general.systemTZ)).toBe(false);
  expect(screen.getByRole("combobox").getAttribute("placeholder")).toBe("Asia/Tokyo");
});
