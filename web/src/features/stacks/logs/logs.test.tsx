// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original localized log clear, filter and resource-switch gestures.

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import { AdapterContext } from "@/data/use-adapter";
import { STACKS } from "@/data/demo/fixtures/stacks";
import { logFixture, settleLogs, wireLog } from "@/data/http/logs.test-support";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { StackLogs } from "./StackLogs";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });
it.each(["en", "de"] as const)("%s clear uses arrival identity and resource changes reset filters", async (locale) => {
  const f = logFixture(), user = userEvent.setup();
  const adapter = { ...createDisconnectedAdapter(), subscribeLogs: f.owner.subscribeLogs };
  const stack = { ...STACKS[0]!, id: "0~one", services: [] };
  const tree = (id: string) => <AdapterContext.Provider value={adapter}><IntlProvider locale={locale === "en" ? "en-US" : "de"} messages={catalogs[locale === "en" ? "en-US" : "de"]} timeZone="UTC"><MemoryRouter initialEntries={["/?service=missing-service"]}><TooltipProvider><StackLogs stack={{ ...stack, id }} /></TooltipProvider></MemoryRouter></IntlProvider></AdapterContext.Provider>;
  const view = render(tree("0~one")); await act(settleLogs);
  act(() => { f.sockets[0]!.send({ ...wireLog(1, "first visible"), timestamp: "2026-05-01T00:00:00Z" }); f.flush(); });
  expect(screen.queryByText("first visible") !== null).toBe(true);
  await user.click(screen.getByRole("button", { name: locale === "en" ? "Clear view" : "Ansicht leeren" }));
  expect(screen.queryByText("first visible") === null).toBe(true);
  act(() => { f.sockets[0]!.send({ ...wireLog(2, "older timestamp arrives later"), timestamp: "2026-01-01T00:00:00Z" }); f.flush(); });
  expect(screen.queryByText("older timestamp arrives later") !== null).toBe(true);
  await user.type(screen.getByRole("searchbox"), "missing");
  view.rerender(tree("0~two")); await act(settleLogs);
  expect((screen.getByRole("searchbox") as HTMLInputElement).value).toBe("");
  act(() => { f.sockets[1]!.send(wireLog(1, "new resource")); f.sockets[1]!.send({ ...wireLog(3, "cut"), truncated: true }); f.flush(); });
  expect(screen.queryByText("new resource") !== null).toBe(true);
  expect(screen.queryAllByText(locale === "en" ? "Unknown source" : "Unbekannte Quelle").length > 0).toBe(true);
  expect(screen.queryByText(locale === "en" ? "[line truncated]" : "[Zeile gekürzt]") !== null).toBe(true);
  expect(screen.queryByText(locale === "en" ? "1 log entry was lost before this line." : "1 Logeintrag ging vor dieser Zeile verloren.") !== null).toBe(true);
  view.unmount(); expect(f.sockets[1]!.close).toHaveBeenCalledTimes(1);
});
