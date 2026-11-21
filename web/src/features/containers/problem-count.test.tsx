// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #556 tests: one problem definition for the container strip and the topology.

import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { isProblem, type RuntimeStatus } from "@/domain/status";
import type { ContainerInfo } from "@/domain/model";
import { catalogs } from "@/app/i18n/catalog";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import ContainersPage from "./ContainersPage";

beforeEach(() => {
  preferencesStore.set((p) => ({ ...p, containersLayout: "table" }));
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});
afterEach(() => { preferencesStore.reset(); simulationStore.reset(); localStorage.clear(); });

it("counts failed, unhealthy and restarting containers, not stopped or unknown ones", () => {
  const all: RuntimeStatus[] = ["running", "unhealthy", "starting", "restarting", "down", "stopped", "unknown"];
  expect(all.filter(isProblem)).toEqual(["unhealthy", "restarting", "down"]);
});

it("the container strip counts a failed container and ignores an ordinary stopped one", async () => {
  const demo = createDemoAdapter();
  const at = "2026-11-21T00:00:00Z";
  // Two containers of one healthy host: one failed (down condition) and one ordinarily stopped.
  const rows = (list: ContainerInfo[]) => {
    const [one, two] = list.filter((c) => c.state === "running");
    return [{ ...one!, id: "failed", state: "stopped" as const, health: "none" as const, condition: { containerId: "failed", condition: "down" as const, reason: "exit", exitCode: 1, sampledAt: at } },
      { ...two!, id: "stopped", name: "stopped-one", state: "stopped" as const, health: "none" as const, condition: undefined }];
  };
  const adapter = { ...demo, listContainers: async (onPartial?: (r: ContainerInfo[]) => void, signal?: AbortSignal) => { const out = rows(await demo.listContainers(undefined, signal)); onPartial?.(out); return out; } };
  renderWithApp(<ContainersPage />, adapter);
  const label = catalogs.de.containers.strip.unhealthy;
  const value = () => screen.getByText(label).closest("[role=group]")?.querySelector("dd > span")?.textContent;
  await waitFor(() => expect(value()).toBe("1"));
});
