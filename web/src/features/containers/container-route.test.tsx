// Source: docklet-next/issues/477
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original tests for container page addresses by name and tab.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it } from "vitest";
import { simulationStore } from "@/data/demo/simulation";
import type { ContainerInfo } from "@/domain/model";
import { renderRoutes } from "@/test/render";
import ContainerPage from "./ContainerPage";
import { findRouteContainer } from "./container-route";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });

const container = (id: string, name: string, hostId: string) => ({ id, name, hostId }) as ContainerInfo;
const list = [container("boreal~1111", "example", "boreal"), container("atlas~2222", "example-two", "atlas"), container("atlas~example", "other", "atlas")];

it.each([
  ["the inventory ID", "boreal~1111", "boreal~1111"],
  ["the exact name on its host", "boreal~example", "boreal~1111"],
  ["an ID before a name", "atlas~example", "atlas~example"],
  ["a name on another host", "boreal~example-two", undefined],
  ["a name substring", "boreal~exam", undefined],
  ["a name without a host", "example", undefined],
])("resolves %s", (_name, ref, id) => {
  expect(findRouteContainer(list, ref)?.id).toBe(id);
});

it("keeps a name address valid after a recreate changes the ID", () => {
  expect(findRouteContainer([container("boreal~3333", "example", "boreal")], "boreal~example")?.id).toBe("boreal~3333");
  expect(findRouteContainer([container("boreal~3333", "example", "boreal")], "boreal~1111")).toBe(undefined);
});

const page = (address: string) => renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], [address]);

it("opens a container page by name and keeps the address", async () => {
  const { router } = page("/containers/boreal~uptime-kuma");
  expect(await screen.findByRole("heading", { level: 1, name: "uptime-kuma" })).toBeTruthy();
  expect(router.state.location.pathname).toBe("/containers/boreal~uptime-kuma");
});

it("shows the empty state for a name that exists only on another host", async () => {
  page("/containers/atlas~uptime-kuma");
  expect(await screen.findByText("Container nicht gefunden")).toBeTruthy();
});

it("opens the terminal tab from the address once a shell is available", async () => {
  const { router } = page("/containers/boreal~uptime-kuma?tab=terminal");
  const terminal = await screen.findByRole("tab", { name: "Terminal" });
  await waitFor(() => expect(terminal.getAttribute("aria-selected")).toBe("true"));
  await userEvent.click(screen.getByRole("tab", { name: "Logs" }));
  await waitFor(() => expect(router.state.location.search).toBe("?tab=logs"));
});

it.each([
  ["a stopped container", "/containers/boreal~whoami-test?tab=terminal"],
  ["an unknown tab", "/containers/boreal~uptime-kuma?tab=files"],
])("falls back to the logs tab for %s", async (_name, address) => {
  page(address);
  const logs = await screen.findByRole("tab", { name: "Logs" });
  await waitFor(() => expect(logs.getAttribute("aria-selected")).toBe("true"));
  expect(screen.getByRole("tab", { name: "Terminal" }).getAttribute("aria-selected")).toBe("false");
});
