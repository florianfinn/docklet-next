import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import { AppShell } from "./AppShell";

type HappyWindow = Window & { happyDOM: { setViewport: (v: { width: number; height: number }) => void } };
const setViewport = (width: number) => act(() => (window as unknown as HappyWindow).happyDOM.setViewport({ width, height: 800 }));

function Page({ name }: { name: string }) {
  return <h1>{name}</h1>;
}

const routes = [
  {
    element: <AppShell />,
    children: [
      { path: "/dashboard", element: <Page name="Dashboard-Seite" /> },
      { path: "/containers", element: <Page name="Container-Seite" /> },
    ],
  },
];

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  setViewport(1280);
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

describe("mobile navigation", () => {
  it("closes the sheet after choosing a page", async () => {
    setViewport(390);
    const { router } = renderRoutes(routes, ["/dashboard"]);
    await userEvent.click(screen.getByRole("button", { name: "Navigation ein- oder ausklappen" }));
    await waitFor(() => expect(document.querySelector('[data-mobile="true"]') !== null).toBe(true));

    const sheet = document.querySelector<HTMLElement>('[data-mobile="true"]');
    const link = [...(sheet?.querySelectorAll("a") ?? [])].find((a) => a.getAttribute("href") === "/containers");
    expect(link !== undefined).toBe(true);
    await userEvent.click(link as HTMLElement);

    expect(router.state.location.pathname).toBe("/containers");
    await waitFor(() => expect(document.querySelector('[data-mobile="true"]') === null).toBe(true));
    expect(screen.getByRole("heading", { name: "Container-Seite" }) !== null).toBe(true);
  });
});

describe("breadcrumbs", () => {
  it("read a volume opened through an encoded link by its decoded identity, like the page", async () => {
    const adapter = createDemoAdapter();
    const reads = vi.spyOn(adapter, "getVolume");
    const id = "first~bind:2f7372762f61";
    renderRoutes([{ element: <AppShell />, children: [{ path: "/volumes/:volumeId", element: <Page name="Volume-Seite" /> }] }], [`/volumes/${encodeURIComponent(id)}`], adapter);
    await screen.findByRole("heading", { name: "Volume-Seite" });
    await waitFor(() => expect(reads).toHaveBeenCalled());
    expect([...new Set(reads.mock.calls.map((c) => c[0]))]).toEqual([id]);
  });
});

it("names the sidebar rail and trigger in the active language", async () => {
  renderRoutes(routes, ["/dashboard"]);
  await screen.findByRole("heading", { name: "Dashboard-Seite" });
  expect(document.body.textContent?.includes("Toggle Sidebar")).toBe(false);
  expect([...document.querySelectorAll("[data-sidebar=rail]")].map((rail) => rail.getAttribute("aria-label"))).toEqual(["Navigation ein- oder ausklappen"]);
});
