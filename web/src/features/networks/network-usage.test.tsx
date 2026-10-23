// Source: docklet-next/issues/204
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original usage indicator gesture tests for issue #204.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import type { Topology, TopologyNetwork } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { NetworkTable } from "./NetworkTable";

const network: TopologyNetwork = { id: "net", hostId: "atlas", name: "example_net", driver: "bridge", internal: false };

function renderTable(patch: Partial<TopologyNetwork> = {}, state?: "running" | "stopped") {
  const member = { ...STANDALONE[0]!, id: "member", name: "example-container", hostId: "atlas", state: state ?? "running" };
  const n = { ...network, ...patch };
  const topology: Topology = { networks: [n], attachments: state ? [{ networkId: n.id, containerId: member.id }] : [], dependencies: [] };
  renderWithApp(<NetworkTable host={HOSTS.find((h) => h.id === "atlas")!} networks={[n]} topology={topology} containers={[member]} focus={null} query="" onClearQuery={() => {}} onFocus={() => {}} onConnect={() => {}} onRemove={() => {}} />);
}

describe("network usage indicator", () => {
  it.each(["running", "stopped"] as const)("counts a %s attachment without an IP and exposes its name and state on keyboard focus", async (state) => {
    renderTable({}, state);
    const trigger = screen.queryByRole("button", { name: "example_net: In Verwendung, 1 Container" });
    expect(trigger !== null).toBe(true);
    // Reach the trigger with the keyboard, not a hover or a programmatic focus.
    for (let n = 0; n < 20 && document.activeElement !== trigger; n++) await userEvent.tab();
    expect(document.activeElement === trigger).toBe(true);
    const detail = await screen.findByRole("tooltip");
    expect(within(detail).getByText(`example-container · ${state === "running" ? "läuft" : "gestoppt"}`) !== null).toBe(true);
    expect(trigger?.textContent).toBe("1");
  });

  it.each([
    [{}, "Unbenutzt", "Keine verbundenen Container"],
    [{ stackId: "stack", stackName: "example-stack" }, "Unbenutzt", "Gehört zum Compose-Stack example-stack"],
    [{ builtIn: true }, "Docker-Standard", "Docker-Standardnetzwerk; kann nicht gelöscht werden."],
  ] as const)("shows attachment-free, Compose and built-in details for %j on hover", async (patch, state, expected) => {
    renderTable(patch);
    await userEvent.hover(screen.getByRole("button", { name: `example_net: ${state}, 0 Container` }));
    expect(within(await screen.findByRole("tooltip")).getByText(expected) !== null).toBe(true);
  });
});
