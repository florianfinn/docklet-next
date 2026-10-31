// Source: docklet-next/issues/313
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original single Unraid ring toggle and ring-only centre counts.

import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import { fleetViewStore, setFleetView } from "./fleet-view-store";
import { OpsSummary } from "./OpsSummary";

afterEach(() => { preferencesStore.reset(); fleetViewStore.reset(); localStorage.clear(); });

const base = STANDALONE.find((item) => item.origin === "standalone")!;

function render() {
  const hosts = HOSTS.filter((host) => host.status === "online").slice(0, 2);
  const containers = hosts.flatMap((host) => [
    { ...base, id: `${host.id}~plain`, hostId: host.id, stackId: undefined, origin: "standalone" as const, state: "running" as const },
    { ...base, id: `${host.id}~unraid`, hostId: host.id, stackId: undefined, origin: "unraid" as const, state: "running" as const },
  ]);
  return renderWithApp(<OpsSummary hosts={hosts} allHostsCount={hosts.length} stacks={[]} containers={containers} attention={[]} attentionText={() => ""} updates={0} critical={undefined} runningActions={0} editing={false} animate={false} duration={0} />);
}

it("W1 per-host rings share one Unraid toggle", async () => {
  setFleetView({ mode: "perHost" });
  render();
  const toggles = screen.getAllByRole("checkbox", { name: "Unraid-Container anzeigen" });
  expect(toggles.length).toBe(1);
  await userEvent.click(toggles[0]!);
  expect(preferencesStore.get().showUnraidRing).toBe(true);
});

it("W1 the joint ring centre counts only the units the ring shows", async () => {
  setFleetView({ mode: "joint" });
  const view = render();
  const centre = () => [...view.container.querySelectorAll("svg text")].map((node) => node.textContent);
  expect(centre().includes("2/2")).toBe(true);
  await userEvent.click(screen.getByRole("checkbox", { name: "Unraid-Container anzeigen" }));
  expect(centre().includes("4/4")).toBe(true);
});
