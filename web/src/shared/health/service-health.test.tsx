// Source: docklet-next/issues/637
// Revision: 24638d9cd8ce431af9ee9b829ec68b171c77e287
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 container and lazy service health gesture tests.

import { useState } from "react";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import * as principal from "@/data/use-principal";
import { simulationStore } from "@/data/demo/simulation";
import { renderStackDetail } from "@/data/message-code";
import type { ContainerHealthDetails } from "@/domain/health";
import { renderRoutes, renderWithApp } from "@/test/render";
import ContainerPage from "@/features/containers/ContainerPage";
import { ServicesCard } from "@/features/stacks/detail/ServicesCard";
import { ServicesHover } from "@/shared/components/services-hover";
import { ServiceHealthHint } from "./ServiceHealthHint";

const raw = 'curl: (22) The requested URL returned error: 404';
const details = (id: string): ContainerHealthDetails => ({ containerId: id, running: true, status: "unhealthy", startedAt: "2026-11-01T10:00:00Z", failingStreak: 2,
  check: { test: ["CMD", "curl", "http://example.com/health"], intervalMs: 0, timeoutMs: 0, retries: 3, startPeriodMs: 0, startIntervalMs: 0 },
  probes: [{ start: "2026-11-01T10:01:00Z", exitCode: 1, output: raw }] });
beforeEach(() => { setAppLocale("en-US"); simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { setAppLocale("de"); simulationStore.reset(); vi.restoreAllMocks(); });
async function setup() {
  const demo = createDemoAdapter();
  const health = vi.fn(async (id: string) => details(id));
  const stack = renderStackDetail(await demo.getStack("media"), "en-US");
  stack.services = [
    { ...stack.services[0]!, name: "web app", containerId: "unhealthy-id", health: "unhealthy" },
    { ...stack.services[0]!, name: "healthy", containerId: "healthy-id", health: "healthy" },
  ];
  return { stack, health, adapter: { ...demo, getContainerHealth: health } };
}

it("keeps the lazy-read guard disabled until open, including across container identity changes", async () => {
  const { adapter, health } = await setup();
  function Preview() {
    const [open, setOpen] = useState(false);
    const [id, setId] = useState("old");
    return <><button onClick={() => setOpen(!open)}>Toggle</button><button onClick={() => setId("new")}>Replace</button>
      <ServiceHealthHint key={id} containerId={id} stackId="media" serviceName="web app" open={open} /></>;
  }
  renderWithApp(<Preview />, adapter);
  const user = userEvent.setup();
  expect(health).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Toggle" }));
  await screen.findByText(/checked path does not exist/);
  await user.click(screen.getByRole("button", { name: "Toggle" }));
  await user.click(screen.getByRole("button", { name: "Replace" }));
  expect(health.mock.calls.map(([id]) => id)).toEqual(["old"]);
  await user.click(screen.getByRole("button", { name: "Toggle" }));
  await waitFor(() => expect(health.mock.calls.map(([id]) => id)).toEqual(["old", "new"]));
});

it("says there is no container instead of loading forever when the service has none", async () => {
  const { adapter, health } = await setup();
  renderWithApp(<ServiceHealthHint containerId={undefined} stackId="media" serviceName="web app" open />, adapter);
  expect([screen.queryByText("No container") !== null, screen.queryByText("Loading health details…") === null]).toEqual([true, true]);
  expect(health).not.toHaveBeenCalled();
});

it("reads only the unhealthy service after opening the overview hover and links to its Health tab", async () => {
  const { adapter, health, stack } = await setup();
  renderWithApp(<ServicesHover stackId={stack.id} services={stack.services} host={undefined} name={stack.name} />, adapter);
  expect(health).not.toHaveBeenCalled();
  const user = userEvent.setup();
  await user.hover(screen.getByRole("button", { name: new RegExp(stack.name) }));
  await screen.findByText(/checked path does not exist/);
  expect(health.mock.calls.map(([id]) => id)).toEqual(["unhealthy-id"]);
  expect(screen.getByRole("link", { name: "Show health" }).getAttribute("href")).toBe(`/stacks/${stack.id}?tab=health&service=web%20app`);
  await user.unhover(screen.getByRole("button", { name: new RegExp(stack.name) }));
  await waitFor(() => expect(screen.queryByRole("link", { name: "Show health" }) === null).toBe(true));
});

it("offers the same lazy hint and link from the ServicesCard status badge", async () => {
  const { adapter, health, stack } = await setup();
  renderWithApp(<ServicesCard stack={stack} host={undefined} />, adapter);
  expect(health).not.toHaveBeenCalled();
  const user = userEvent.setup();
  const trigger = screen.getByRole("button", { name: "web app: Unhealthy" });
  trigger.focus();
  await user.keyboard("{Enter}");
  await screen.findByText(/checked path does not exist/);
  expect(health.mock.calls.map(([id]) => id)).toEqual(["unhealthy-id"]);
  expect(screen.getByRole("link", { name: "Show health" }).getAttribute("href")).toBe(`/stacks/${stack.id}?tab=health&service=web%20app`);
});

it("keeps overview previews without admin context free of health reads and links", async () => {
  const { adapter, health, stack } = await setup();
  renderWithApp(<ServicesHover services={stack.services} host={undefined} name={stack.name} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: new RegExp(stack.name) }));
  expect(screen.queryByRole("link", { name: "Show health" }) === null).toBe(true);
  expect(health).not.toHaveBeenCalled();
});

it("renders the container Health card above availability with a keyboard raw toggle", async () => {
  const { adapter, health } = await setup();
  const { container } = renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  await screen.findByText(/checked path does not exist/);
  expect(screen.getByText("Health").textContent).toBe("Health");
  expect(screen.getByText("Failing streak: 2").textContent).toBeTruthy();
  expect(health.mock.calls.map(([id]) => id)).toEqual(["c-uptime-kuma"]);
  const user = userEvent.setup();
  const toggle = screen.getByRole("button", { name: "Raw health data" });
  toggle.focus();
  await user.keyboard("{Enter}");
  expect(container.querySelector("pre")?.textContent).toBe(raw);
  await user.keyboard("{Enter}");
  expect(container.querySelector("pre") === null).toBe(true);
});


it("keeps simple-user ServicesCard badges free of health navigation", async () => {
  vi.spyOn(principal, "useIsUser").mockReturnValue(true);
  const { adapter, health, stack } = await setup();
  renderWithApp(<ServicesCard stack={stack} host={undefined} />, adapter);
  expect(screen.queryByRole("button", { name: "web app: Unhealthy" }) === null).toBe(true);
  expect(screen.queryByRole("link", { name: "Show health" }) === null).toBe(true);
  expect(health).not.toHaveBeenCalled();
});

it("retains navigation when a lazy health read fails", async () => {
  const { adapter, stack } = await setup();
  adapter.getContainerHealth = vi.fn(async () => { throw new Error("Health read unavailable"); });
  renderWithApp(<ServicesHover stackId={stack.id} services={stack.services} host={undefined} name={stack.name} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: new RegExp(stack.name) }));
  expect((await screen.findByRole("alert")).textContent).toBe("Health details unavailable");
  expect(screen.getByRole("link", { name: "Show health" }).getAttribute("href")).toBe(`/stacks/${stack.id}?tab=health&service=web%20app`);
});
