import { act, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { IntlProvider } from "use-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { routes } from "@/app/router";
import { simulationStore } from "@/data/demo/simulation";
import { formatCountdown, formatMem, formatPercent, formatRelative } from "@/domain/format";
import { formatPercentNumber, withPercent } from "@/domain/locale";
import LoginPage from "@/features/auth/login/LoginPage";
import { FlowsSection } from "@/features/design-lab/ControlsSection";
import SetupPage from "@/features/setup/SetupPage";
import { restartSetup } from "@/features/setup/setup-draft";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { renderRoutes, renderWithApp } from "@/test/render";

import { App } from "../App";
import { catalogs } from "./catalog";
import { localeStore, setAppLocale, type AppLocale } from "./locale-store";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => {
  simulationStore.reset();
  localeStore.reset();
  localStorage.clear();
});

const switchTo = (name: "English (US)" | "Deutsch") => userEvent.click(within(screen.getAllByRole("radiogroup", { name: /^(Sprache|Language)$/ })[0]!).getByRole("radio", { name }));

describe("language switch", () => {
  it("starts in German and sets document language and title for the chosen language", () => {
    render(<App />);
    expect([localeStore.get(), document.documentElement.lang, document.title]).toEqual(["de", "de", "Docklet · React-Prototyp"]);
    act(() => setAppLocale("en-US"));
    expect([document.documentElement.lang, document.title]).toEqual(["en-US", "Docklet · React prototype"]);
  });

  it("switches the login page before the shell, persists the choice and returns to the unchanged German texts", async () => {
    renderRoutes([{ path: "/login", element: <LoginPage /> }], ["/login"]);
    const germanTitle = await screen.findByRole("heading", { level: 1 });
    expect(germanTitle.textContent).toBe("Bei Docklet anmelden");
    expect(screen.getByRole("radio", { name: "Deutsch" }).getAttribute("aria-checked")).toBe("true");

    await switchTo("English (US)");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Sign in to Docklet");
    expect(screen.getByRole("radiogroup", { name: "Language" }) !== null).toBe(true);
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.locale") ?? "{}")).toEqual({ v: 1, data: "en-US" });

    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(screen.getAllByText("Required").length).toBe(2);

    await switchTo("Deutsch");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Bei Docklet anmelden");
    expect(screen.getAllByText("Pflichtfeld").length).toBe(2);
  });

  it("runs first-run setup in English, including validation and help buttons", async () => {
    restartSetup();
    renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"]);
    await switchTo("English (US)");
    await waitFor(() => expect((screen.getByLabelText("Internal hub address") as HTMLInputElement).value).toBe("192.0.2.10:8080"));
    await userEvent.clear(screen.getByLabelText("Internal hub address"));
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("Check 1 field") !== null).toBe(true);
    expect(screen.getByRole("button", { name: "Explanation: Internal hub address" }) !== null).toBe(true);
  });

  it("offers the switch in hub settings as a browser preference and in the design lab flows", async () => {
    setAppLocale("en-US");
    const { router } = renderRoutes(routes, ["/settings/general"]);
    expect(await screen.findByText("Applies to this browser only, right away; not a hub setting.") !== null).toBe(true);
    await switchTo("Deutsch");
    expect(screen.getByText("Gilt nur für diesen Browser und sofort; keine Hub-Einstellung.") !== null).toBe(true);
    expect(router.state.location.pathname).toBe("/settings/general");

    renderWithApp(<FlowsSection />);
    await userEvent.click(within(screen.getAllByRole("radiogroup", { name: "Sprache" }).at(-1)!).getByRole("radio", { name: "English (US)" }));
    expect(localeStore.get()).toBe("en-US");
  });
});

describe("formatting per language", () => {
  const now = Date.parse("2026-10-12T10:00:00Z");
  const iso = (ms: number) => new Date(now + ms).toISOString();
  const formats = () => [formatRelative(iso(-10_000), now), formatRelative(iso(-2 * 3_600_000), now), formatCountdown(iso(3 * 86_400_000), now), formatMem(1536), formatPercent(42), formatPercentNumber(12.5), withPercent(30)];

  it("keeps German output exactly as before and uses American English conventions for en-US", () => {
    expect(formats()).toEqual(["gerade eben", "vor 2 Stunden", "in 3 Tagen", "1,5 GB", "42 %", "12,5 %", "30 %"]);
    setAppLocale("en-US");
    expect(formats()).toEqual(["just now", "2 hours ago", "in 3 days", "1.5 GB", "42%", "12.5%", "30%"]);
  });

  it("formats schedules in the hub time zone whatever the language", () => {
    const wrapper = (locale: AppLocale) =>
      function Wrapper({ children }: { children: ReactNode }) {
        return (
          <IntlProvider locale={locale} messages={catalogs[locale]} timeZone="Europe/Berlin">
            {children}
          </IntlProvider>
        );
      };
    const next = (locale: AppLocale) => {
      setAppLocale(locale);
      const { result } = renderHook(() => useScheduleText(), { wrapper: wrapper(locale) });
      return result.current.next({ mode: "daily", time: "03:00" }, new Date(now), "Europe/Berlin");
    };
    const de = next("de");
    const en = next("en-US");
    expect(de?.at.toISOString()).toBe("2026-10-13T01:00:00.000Z");
    expect(en?.at.toISOString()).toBe(de?.at.toISOString());
    expect([de?.when, de?.zone, de?.relative]).toEqual(["Di., 13.10.2026, 03:00", "Europe/Berlin (MESZ)", "in 15 Stunden"]);
    expect([en?.when, en?.zone, en?.relative]).toEqual(["Tue, 10/13/2026, 03:00 AM", "Europe/Berlin (GMT+2)", "in 15 hours"]);
  });
});

// Last in the file: fresh module instances must not mix with the routes rendered above.
describe("after a reload", () => {
  it("starts in the stored language and falls back to German for unknown values", async () => {
    localStorage.setItem("docklet-react-opus.locale", JSON.stringify({ v: 1, data: "en-US" }));
    vi.resetModules();
    expect((await import("./locale-store")).localeStore.get()).toBe("en-US");
    localStorage.setItem("docklet-react-opus.locale", JSON.stringify({ v: 1, data: "fr" }));
    vi.resetModules();
    expect((await import("./locale-store")).localeStore.get()).toBe("de");
  });
});
