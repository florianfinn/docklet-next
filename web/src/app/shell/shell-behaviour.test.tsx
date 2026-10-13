import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

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
