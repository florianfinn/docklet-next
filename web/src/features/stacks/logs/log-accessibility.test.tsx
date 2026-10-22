// Source: docklet-next/docs/design/runtime-control.md
// Revision: ed78cd8cf48b88e8225b656b0976d248829f85eb
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original keyboard and screen-reader checks on both log surfaces.

import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { localeStore } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { logFixture, settleLogs, wireLog } from "@/data/http/logs.test-support";
import ContainerPage from "@/features/containers/ContainerPage";
import { renderRoutes } from "@/test/render";
import { StackLogs } from "./StackLogs";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); simulationStore.reset(); localStorage.clear(); });
it.each(["en-US", "de"] as const)("%s source streams are readable and keyboard discoverable on both surfaces", async (locale) => {
  localeStore.set(locale);
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const base = createDemoAdapter();
  const stack = (await base.listStacks())[0]!;
  const container = (await base.listContainers())[0]!;
  for (const kind of ["stack", "container"] as const) {
    const f = logFixture(); const user = userEvent.setup();
    const adapter = { ...base,
      subscribeLogs: (_id: string, push: Parameters<typeof f.owner.subscribeLogs>[1]) => f.owner.subscribeLogs("0~example", push),
      subscribeContainerLogs: (_id: string, push: Parameters<typeof f.owner.subscribeContainerLogs>[1]) => f.owner.subscribeContainerLogs("0~example", push),
    };
    const view = renderRoutes([
      { path: "/stack", element: <StackLogs stack={stack} /> },
      { path: "/containers/:containerId", element: <ContainerPage /> },
    ], [kind === "stack" ? "/stack" : `/containers/${container.id}`], adapter);
    await screen.findByRole("log"); await act(settleLogs);
    act(() => {
      f.sockets[0]!.send(Array.from({ length: 100 }, (_, i) => ({ ...wireLog(i + 1, i === 0 ? "visible application text" : `line ${i}`), service: "example-source", stream: "stderr" })));
      f.flush();
    });
    const region = screen.getByRole("log");
    const sources = within(region).getAllByText("example-source");
    const label = locale === "de" ? "Standardfehlerkanal (kein Schweregrad)" : "Standard error stream (not a severity)";
    expect(sources.length === 100).toBe(true);
    expect(sources.every((source) => source.querySelector(".sr-only")?.textContent?.includes(label) && !source.closest('[aria-hidden="true"]'))).toBe(true);
    expect(sources.every((source) => source.closest(".select-none") !== null)).toBe(true);
    const before = document.createElement("button"), after = document.createElement("button");
    region.before(before); region.after(after); before.focus();
    await user.tab(); expect(document.activeElement === region).toBe(true);
    await user.tab(); expect(document.activeElement === after).toBe(true);
    await user.hover(sources[0]!);
    expect((await screen.findByRole("tooltip")).textContent?.includes(label)).toBe(true);
    await user.unhover(sources[0]!);
    before.remove(); after.remove();
    expect(screen.queryByText("visible application text") !== null).toBe(true);
    await user.click(screen.getByRole("button", { name: kind === "stack" ? (locale === "de" ? "Pausieren" : "Pause") : (locale === "de" ? "Logs anhalten" : "Pause logs") }));
    const hasConditionalNotice = () => screen.getAllByRole("status").some((notice) => notice.textContent?.includes(locale === "de" ? "nur neue Zeilen" : "only new lines"));
    expect(hasConditionalNotice()).toBe(true);
    await user.click(screen.getByRole("button", { name: kind === "stack" ? (locale === "de" ? "Fortsetzen" : "Resume") : (locale === "de" ? "Logs fortsetzen" : "Resume logs") }));
    await act(settleLogs);
    act(() => { f.sockets[1]!.send({ ...wireLog(8, "future line only"), service: "example-source" }); f.flush(); });
    expect(hasConditionalNotice()).toBe(true);
    expect(screen.queryByText("visible application text") === null).toBe(true);
    expect(screen.queryByText("future line only") !== null).toBe(true);
    view.unmount();
  }
});
