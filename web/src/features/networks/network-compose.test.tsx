// Source: docklet-next/issues/204
// Revision: d03b817a6aa2286616ba32fc3b04a9d3f82422ed
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original external Compose ownership and known stack link regression tests.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { networkFixture } from "@/data/http/network-commands.test-support";
import { resourceFixture } from "@/data/http/resources.test-support";
import { networkComposeOwner, removeBlock } from "@/domain/networks";
import { renderWithApp } from "@/test/render";
import { NetworkTable } from "./NetworkTable";
import { TopologyDetails } from "./TopologyDetails";

describe.each([false, true])("Compose network ownership (Edge=%s)", (edge) => {
  it.each([
    [{ "com.docker.compose.project": "external" }, "external"],
    [{ "com.docker.compose.network": "default" }, "default"],
    [{ "com.docker.compose.project": "external", "com.docker.compose.network": "default" }, "external"],
  ] as const)("protects and identifies external ownership from %j", async (labels, owner) => {
    const f = networkFixture(edge); f.state.networks[0]!.labels = labels;
    const topology = await f.adapter.getTopology(); const hosts = await f.adapter.listHosts();
    const network = topology.networks.find((n) => n.id === "host~net1")!;
    expect(network.stackId).toBeUndefined();
    expect(removeBlock(topology, network)).toBe("compose-managed");
    expect(networkComposeOwner(network)).toBe(owner);
    renderWithApp(<NetworkTable host={hosts[0]!} networks={[network]} topology={topology} containers={[]} focus={null} query="" onClearQuery={() => {}} onFocus={() => {}} onConnect={() => {}} onRemove={() => {}} />, f.adapter);
    const trigger = screen.getByRole("button", { name: "example_net: Unbenutzt, 0 Container" });
    for (let i = 0; i < 20 && document.activeElement !== trigger; i++) await userEvent.tab();
    expect(document.activeElement === trigger).toBe(true);
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip.textContent?.includes(`Gehört zum Compose-Stack ${owner}`)).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Aktionen für example_net" }));
    expect(screen.getByRole("menuitem", { name: "Entfernen" }).getAttribute("aria-disabled")).toBe("true");
  });
});

it("preserves the known hub stack identity and link", async () => {
  const f = resourceFixture(); const topology = await f.adapter.getTopology(); const hosts = await f.adapter.listHosts();
  const network = topology.networks.find((n) => n.hostId === "first")!;
  expect(network).toMatchObject({ stackId: "first~project", stackName: "app" });
  expect(networkComposeOwner(network)).toBe("app");
  expect(removeBlock(topology, network)).toBe("compose-managed");
  renderWithApp(<TopologyDetails selected={network.id} topology={topology} containers={[]} hosts={new Map(hosts.map((h) => [h.id, h]))} onSelect={() => {}} expanded={new Set()} onToggleExpand={() => {}} onConnect={() => {}} onRemove={() => {}} />, f.adapter);
  expect(screen.getByRole("link", { name: "Stack öffnen" }).getAttribute("href")).toBe("/stacks/first~project");
  expect(within(screen.getByLabelText("front")).getByText("Stack app") !== null).toBe(true);
});
