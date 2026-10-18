// Source: docklet-next/issues/85
// Revision: a97d94423e731203eaf5bdcfe96e7744ce592fa2
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original stack ID fallback search and tooltip tests.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { filterImages } from "@/features/images/image-filters";
import { filterVolumes } from "@/features/volumes/volume-filters";
import { NetworkTable } from "@/features/networks/NetworkTable";
import { TopologyDetails } from "@/features/networks/TopologyDetails";
import { decorate, layoutHost } from "@/features/networks/topology-layout";
import { renderWithApp } from "@/test/render";
import { resourceFixture } from "./resources.test-support";

afterEach(() => localStorage.clear());

it("A-N4 searches images and volumes by stackId when the name is missing", async () => {
 const { adapter } = resourceFixture();
 const images = (await adapter.listImages()).map((image) => ({ ...image, consumers: image.consumers.map((c) => ({ ...c, stackName: undefined })) }));
 const volumes = (await adapter.listVolumes()).map((v) => ({ ...v, stackName: undefined }));
 expect(filterImages(images, { query: " FIRST~PROJECT ", usage: "all" }).map((i) => i.id)).toEqual(["first~sha256:user"]);
 expect(filterVolumes(volumes, { query: " FIRST~PROJECT ", kind: "all", usage: "all" }).map((v) => v.id)).toEqual(["first~data"]);
});

it("A-N4 searches network members and highlights topology containers by stackId", async () => {
 const { adapter } = resourceFixture();
 const host = (await adapter.listHosts())[0]!;
 const containers = (await adapter.listContainers()).map((c) => ({ ...c, stackName: undefined }));
 const topology = await adapter.getTopology();
 const networks = topology.networks.filter((n) => n.hostId === host.id);
 const layout = layoutHost({ host, containers, topology, expanded: new Set(networks.map((n) => n.id)) });
 const { nodes } = decorate(layout, { focus: null, query: "first~project", onlyProblems: false, showDependencies: true });
 const members = nodes.filter((n) => n.type === "container");
 expect(members.length).toBeGreaterThan(0);
 expect(members.every((n) => !n.data.dim)).toBe(true);
 renderWithApp(<NetworkTable host={host} networks={networks} topology={topology} containers={containers} focus={null} query="first~project" onClearQuery={() => {}} onFocus={() => {}} onConnect={() => {}} onRemove={() => {}} />, adapter);
 expect(screen.getByRole("button", { name: "front in der Topologie fokussieren" }) !== null).toBe(true);
});

it("A-N4 includes stackId in the network removal tooltip when the name is missing", async () => {
 const { adapter } = resourceFixture();
 const hosts = new Map((await adapter.listHosts()).map((h) => [h.id, h]));
 const original = await adapter.getTopology();
 const topology = { ...original, networks: original.networks.map((n) => ({ ...n, stackName: undefined })) };
 renderWithApp(<TopologyDetails selected="first~net" topology={topology} containers={await adapter.listContainers()} hosts={hosts} expanded={new Set()} onSelect={() => {}} onToggleExpand={() => {}} onConnect={() => {}} onRemove={() => {}} />, adapter);
 const remove = screen.getByRole("button", { name: "Netzwerk entfernen" });
 await userEvent.hover(remove.parentElement!);
 await waitFor(() => expect(screen.getByRole("tooltip").textContent).toContain("first~project"));
});
