import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { AppShell } from "@/app/shell/AppShell";
import { simulationStore } from "@/data/demo/simulation";
import { openActivity } from "@/features/activity/activity-store";
import StackPage from "@/features/stacks/detail/StackPage";
import StacksPage from "@/features/stacks/overview/StacksPage";
import { renderRoutes } from "@/test/render";

const openPopups = () => document.querySelectorAll('[data-slot="popover-content"]').length;

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

describe("info popups", () => {
  it("keep only one popup open when another one is opened", async () => {
    renderRoutes([{ element: <AppShell />, children: [{ path: "/stacks", element: <StacksPage /> }] }], ["/stacks"]);
    const media = await screen.findByRole("button", { name: /^Update-Status für Media/ });
    await userEvent.click(media);
    await waitFor(() => expect(openPopups()).toBe(1));

    await userEvent.click(screen.getByRole("button", { name: /^Update-Status für Cloud/ }));
    await waitFor(() => expect(screen.getByRole("button", { name: /^Update-Status für Cloud/ }).getAttribute("aria-expanded")).toBe("true"));
    expect(openPopups()).toBe(1);
    expect(media.getAttribute("aria-expanded")).toBe("false");

    await userEvent.click(screen.getByRole("button", { name: /Media: \d von \d Services laufen/ }));
    await waitFor(() => expect(openPopups()).toBe(1));
  });

  it("close on a tab change and when the action centre opens", async () => {
    const { router } = renderRoutes([{ element: <AppShell />, children: [{ path: "/stacks/:stackId", element: <StackPage /> }] }], ["/stacks/media"]);
    const header = await screen.findByRole("button", { name: /^Update-Status für Media/ });
    await userEvent.click(header);
    await waitFor(() => expect(openPopups()).toBe(1));

    await act(() => router.navigate("/stacks/media?tab=logs"));
    await waitFor(() => expect(openPopups()).toBe(0));
    expect(screen.getByRole("button", { name: /^Update-Status für Media/ }) === header).toBe(true);

    await userEvent.click(header);
    await waitFor(() => expect(openPopups()).toBe(1));
    act(() => openActivity());
    await waitFor(() => expect(openPopups()).toBe(0));
  });
});
