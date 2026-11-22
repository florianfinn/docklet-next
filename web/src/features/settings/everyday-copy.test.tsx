// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #560 tests: personal scope line, named stack link and the local host card.

import { screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { mapHost } from "@/data/http/hosts";
import type { ContainerInfo } from "@/domain/model";
import ContainerPage from "@/features/containers/ContainerPage";
import { HostCard } from "@/features/hosts/HostCard";
import { renderRoutes, renderWithApp } from "@/test/render";
import SettingsPage from "./SettingsPage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });
const de = catalogs.de;

it("appearance and explanations name the personal scope; hub sections keep the hub scope", async () => {
  for (const [section, title, text] of [["appearance", de.settings.personalTitle, de.settings.personalDescription], ["help", de.settings.personalTitle, de.settings.personalDescription], ["general", de.settings.title, de.settings.description]] as const) {
    const view = renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], [`/settings/${section}`]);
    expect((await screen.findByRole("heading", { level: 1 })).textContent).toBe(title);
    expect(screen.queryAllByText(text).length > 0).toBe(true);
    view.unmount();
  }
});

it("the container page names the stack the service belongs to", async () => {
  const demo = createDemoAdapter();
  const adapter = { ...demo, listContainers: async (onPartial?: (r: ContainerInfo[]) => void, signal?: AbortSignal) => {
    const rows = (await demo.listContainers(undefined, signal)).map((c) => (c.stackId ? { ...c, stackName: "example-stack" } : c));
    onPartial?.(rows); return rows;
  } };
  const owned = (await demo.listContainers()).find((c) => c.stackId && c.serviceName)!;
  renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], [`/containers/${owned.id}`], adapter);
  await screen.findAllByText(owned.name);
  expect(screen.queryByRole("link", { name: `Service ${owned.serviceName} im Stack example-stack` }) !== null).toBe(true);
});

it("the local host card names the hub's own Docker and shows no agent report", () => {
  const local = mapHost({ id: "0", name: "Local", color: null, registrationRevision: 1, apiUrl: "", enabled: true, status: "online", isEdge: false });
  renderWithApp(<HostCard host={local} system={undefined} stacks={1} containers={1} allIds={["0"]} />);
  expect(screen.queryByText(de.hosts.mode.localLong.replace("''", "'")) !== null).toBe(true);
  expect(screen.queryByRole("region", { name: de.hosts.card.report.title }) === null).toBe(true);
});
