// Source: docklet-next/issues/102
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original visible activity source-health and real gesture regressions.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import { activityFixture } from "@/data/http/activity-reader.test-support";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";

afterEach(() => act(() => activityStore.reset()));
function mount(f: ReturnType<typeof activityFixture>, locale: "en-US" | "de") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  act(() => activityStore.set((s) => ({ ...s, open: true, tab: "actions" })));
  return render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={client}><IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><MemoryRouter><ActivityCenter /></MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
}
for (const locale of ["en-US", "de"] as const) {
  it(`${locale}: shows healthy runs and source failure, survives hide/details gestures and clears after retry`, async () => {
    const f = activityFixture(true); f.state.failure = "page403";
    const view = mount(f, locale);
    const user = userEvent.setup();
    const warning = await screen.findByRole("alert");
    expect(warning.textContent?.includes(locale === "en-US" ? "Access to this source was denied." : "Der Zugriff auf diese Quelle wurde verweigert.")).toBe(true);
    expect(warning.textContent?.includes("Unavailable example")).toBe(true);
    expect(warning.textContent?.includes(locale === "en-US" ? "does not confirm completion" : "bestätigt keinen Abschluss")).toBe(true);
    expect(warning.textContent?.includes("403")).toBe(false);
    expect(screen.queryByText(/Example good/) !== null).toBe(true);
    await user.click(screen.getByRole("button", { name: locale === "en-US" ? "Hide completed" : "Abgeschlossene ausblenden" }));
    expect(screen.queryByRole("alert") !== null).toBe(true);
    await user.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.queryByRole("button", { name: locale === "en-US" ? "Open in progress window" : "Im Ablauffenster öffnen" }) !== null).toBe(true);
    f.state.failure = "";
    await user.click(within(warning).getByRole("button", { name: locale === "en-US" ? "Retry activity reads" : "Aktivitäten erneut lesen" }));
    await waitFor(() => expect(screen.queryByRole("alert") === null).toBe(true));
    expect(screen.queryByText(/Example bad/) !== null).toBe(true);
    view.unmount();
  });
  it(`${locale}: unavailable with zero runs is distinct from a successful empty read`, async () => {
    const f = activityFixture(); f.state.lists.good = []; f.state.failure = "page502";
    const view = mount(f, locale); const user = userEvent.setup();
    const warning = await screen.findByRole("alert");
    const empty = locale === "en-US" ? "No actions in this session yet." : "Noch keine Aktionen in dieser Sitzung.";
    expect(screen.queryByText(empty) === null).toBe(true);
    f.state.lists.bad = []; f.state.failure = "";
    await user.click(within(warning).getByRole("button", { name: locale === "en-US" ? "Retry activity reads" : "Aktivitäten erneut lesen" }));
    await screen.findByText(empty);
    expect(screen.queryByRole("alert") === null).toBe(true);
    view.unmount();
  });
}
it("retained rows still show a global refresh failure and reconnect clears it", async () => {
  const f = activityFixture(); const view = mount(f, "en-US");
  await screen.findByText(/Example good/);
  act(() => f.sources[0]!.open());
  await waitFor(() => expect(f.requests.filter((path) => path.includes("/activities/run-good")).length).toBeGreaterThan(1));
  f.state.hubStatus = 500;
  act(() => f.sources[0]!.open());
  await screen.findByRole("alert");
  expect(screen.queryByText(/Example good/) !== null).toBe(true);
  expect(screen.queryByText(/Activity sources/) !== null).toBe(true);
  f.state.hubStatus = 200;
  act(() => f.sources[0]!.open());
  await waitFor(() => expect(screen.queryByRole("alert") === null).toBe(true));
  view.unmount();
});
