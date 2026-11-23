// Source: docklet-next/issues/581
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original rendered consumer proofs for left-out Unraid containers (M37 #581).

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { TOPOLOGY } from "@/data/demo/fixtures/topology";
import type { ContainerInfo } from "@/domain/model";
import type { DockerImage } from "@/domain/images";
import DashboardPage from "@/features/dashboard/DashboardPage";
import { ImageDetails } from "@/features/images/ImageDetails";
import { NetworkTable } from "@/features/networks/NetworkTable";
import { TopologyDetails } from "@/features/networks/TopologyDetails";
import { useHiddenUnraidIds, useVisibleContainers } from "@/shared/hooks/use-host-scope";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp, renderRoutes } from "@/test/render";
import { FileAside } from "@/features/volumes/files/FileAside";
import { AdminSearchGroups } from "@/app/shell/AdminSearchGroups";
import { Command, CommandList } from "@/shared/ui/command";

import HostsPage from "./HostsPage";
import { resetUnraidDetection } from "./unraid-detection";

afterEach(() => { preferencesStore.reset(); localStorage.clear(); resetUnraidDetection(); setAppLocale("de"); });

const atlas = HOSTS.find((h) => h.id === "atlas")!;
const br0 = TOPOLOGY.networks.find((n) => n.id === "n-atlas-br0")!;

it("explains left-out members in the network usage tooltip instead of claiming none are attached", async () => {
  setAppLocale("en-US");
  preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] }));
  renderWithApp(<NetworkTable host={atlas} networks={[br0]} topology={TOPOLOGY} containers={[]} focus={null} query="" onClearQuery={() => {}} onFocus={() => {}} onConnect={() => {}} onRemove={() => {}} />);
  await userEvent.hover(await screen.findByRole("button", { name: "br0: In use, 0 containers" }));
  const tip = await screen.findByRole("tooltip");
  expect(within(tip).queryByText("No attached containers") === null).toBe(true);
  expect(within(tip).getByText("1 left-out Unraid container also connected") !== null).toBe(true);
});

it("lists a network's attachments without left-out containers and says how many are connected besides", async () => {
  setAppLocale("en-US");
  const attached = TOPOLOGY.attachments.filter((a) => a.networkId === br0.id);
  expect(attached.length > 0).toBe(true);
  const view = renderWithApp(<TopologyDetails selected={br0.id} topology={TOPOLOGY} containers={[]} hosts={new Map([[atlas.id, atlas]])} onSelect={() => {}} expanded={new Set()} onToggleExpand={() => {}} onConnect={() => {}} onRemove={() => {}} />);
  await waitFor(() => expect(view.container.textContent?.includes(attached[0]!.containerId)).toBe(true));
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(view.container.textContent?.includes(attached[0]!.containerId)).toBe(false));
  expect(screen.getByText(`${attached.length} left-out Unraid ${attached.length === 1 ? "container" : "containers"} also connected`) !== null).toBe(true);
});

it("leaves left-out consumers out of the image details while removal stays blocked", async () => {
  setAppLocale("en-US");
  const image = { ...(await createDemoAdapter().listImages()).find((i) => i.hostId === "atlas")!, consumers: [{ containerId: "c-duplicati", name: "duplicati", hostId: "atlas", state: "running" }] } as unknown as DockerImage;
  const view = renderWithApp(<ImageDetails id={image.id} image={image} host={atlas} onClose={() => {}} onPull={() => {}} onRemove={() => {}} />);
  await waitFor(() => expect(document.body.textContent?.includes("duplicati")).toBe(true));
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(document.body.textContent?.includes("duplicati")).toBe(false));
  expect(screen.getByText("1 hidden") !== null).toBe(true);
  // The image still reads as used: no "unused" claim replaces the hidden consumer.
  expect(document.body.textContent?.includes("Not used by any container")).toBe(false);
  view.unmount();
});

it("drops the host's Unraid segment from the dashboard ring and names the hidden count", async () => {
  setAppLocale("en-US");
  const view = renderWithApp(<DashboardPage />);
  const segments = () => view.container.querySelectorAll('[data-slot="fleet-ring"] [data-segment]').length;
  await waitFor(() => expect(segments() > 0).toBe(true));
  const before = segments();
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(segments()).toBe(before - 1));
  expect(await screen.findByText("2 Unraid containers hidden") !== null).toBe(true);
});

it("detects and hides Unraid containers of two hosts independently", async () => {
  setAppLocale("en-US");
  const demo = createDemoAdapter();
  const borealUnraid: ContainerInfo = { ...STANDALONE[0]!, id: "c-boreal-unraid", name: "boreal-unraid-app", hostId: "boreal", origin: "unraid", stackId: undefined };
  const adapter = { ...demo, listContainers: async () => [...(await demo.listContainers()), borealUnraid] };
  let names: string[] = [];
  function Probe() { names = useVisibleContainers().visible.map((c) => c.name); return null; }
  const view = renderWithApp(<><Probe /><HostsPage /></>, adapter);
  await waitFor(() => expect(view.container.querySelectorAll('[data-slot="unraid-switch"]').length).toBe(2));
  expect([...view.container.querySelectorAll("[data-host]")].filter((card) => card.querySelector('[data-slot="unraid-switch"]')).map((card) => card.getAttribute("data-host")).sort()).toEqual(["atlas", "boreal"]);
  await userEvent.click(view.container.querySelector<HTMLElement>('[data-host="boreal"] [role="switch"]')!);
  await waitFor(() => expect(names.includes("boreal-unraid-app")).toBe(false));
  expect(names.includes("duplicati") && names.includes("krusader")).toBe(true);
  await userEvent.click(view.container.querySelector<HTMLElement>('[data-host="atlas"] [role="switch"]')!);
  await userEvent.click(view.container.querySelector<HTMLElement>('[data-host="boreal"] [role="switch"]')!);
  await waitFor(() => expect(names.includes("boreal-unraid-app")).toBe(true));
  expect(names.includes("duplicati") || names.includes("krusader")).toBe(false);
  expect(preferencesStore.get().unraidHidden).toEqual(["atlas"]);
});

it("leaves left-out consumers out of the file details and names how many are hidden", async () => {
  setAppLocale("en-US");
  const demo = createDemoAdapter();
  const volume = { ...(await demo.listVolumes())[0]!, consumers: [{ containerId: "c-duplicati", containerName: "duplicati", target: "/data", readOnly: false, running: true }, { containerId: "c-plain", containerName: "plain-app", target: "/srv", readOnly: true, running: true }] };
  const props = { mode: "details", volume, path: "/", active: undefined, files: [], secondary: null, onSecondary: () => {}, onEdit: () => {}, onSave: () => {}, onDownload: () => {} } as unknown as Parameters<typeof FileAside>[0];
  const view = renderWithApp(<FileAside {...props} />);
  expect(within(view.container).queryByRole("link", { name: "duplicati" }) !== null).toBe(true);
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(within(view.container).queryByRole("link", { name: "duplicati" }) === null).toBe(true));
  expect(within(view.container).queryByRole("link", { name: "plain-app" }) !== null).toBe(true);
  expect(view.container.textContent?.includes("1 hidden")).toBe(true);
});

it("offers no left-out container in the global search", async () => {
  setAppLocale("en-US");
  const view = renderWithApp(<Command shouldFilter={false}><CommandList><AdminSearchGroups query="duplicati" select={() => {}} hostName={(id) => id} pages={[]} actions={[]} stacks={[]} stacksLoading={false} /></CommandList></Command>);
  await waitFor(() => expect(within(view.container).queryByText("duplicati") !== null).toBe(true));
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(within(view.container).queryByText("duplicati") === null).toBe(true));
});

it("lists only shown consumers on the volume overview while the volume stays in use", async () => {
  setAppLocale("en-US");
  const demo = createDemoAdapter();
  const base = (await demo.listVolumes()).find((v) => v.hostId === "atlas")!;
  const volume = { ...(await demo.getVolume(base.id)), consumers: [{ containerId: "c-duplicati", containerName: "duplicati", target: "/data", readOnly: false, running: true }] };
  const adapter = { ...demo, getVolume: async () => volume } as typeof demo;
  preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] }));
  const VolumePage = (await import("@/features/volumes/VolumePage")).default;
  const view = renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }], [`/volumes/${encodeURIComponent(base.id)}`], adapter);
  await waitFor(() => expect(view.container.textContent?.includes("1 hidden")).toBe(true));
  expect(view.container.textContent?.includes("duplicati")).toBe(false);
  expect(view.container.textContent?.includes("No container mounts this entry.")).toBe(false);
});

it("offers no left-out container as a proxy destination", async () => {
  setAppLocale("en-US");
  const { DestinationFields } = await import("@/features/proxy/DestinationFields");
  const { instance } = await import("@/features/proxy/proxy.test-support");
  let hidden = false;
  function Probe() { hidden = useHiddenUnraidIds().has("c-duplicati"); return null; }
  preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] }));
  const view = renderWithApp(<><Probe /><DestinationFields instance={{ ...instance, environmentId: "atlas" }} backend={{ scheme: "http", host: "", port: "", colon: false, rest: "" }} update={() => {}} /></>);
  await waitFor(() => expect(hidden).toBe(true));
  await userEvent.click(screen.getByRole("radio", { name: /Container/ }));
  document.getElementById("destination-container")!.focus();
  await userEvent.keyboard("{ArrowDown}");
  const options = await screen.findAllByRole("option");
  expect(options.length > 0).toBe(true);
  expect(options.some((o) => o.textContent?.includes("duplicati"))).toBe(false);
  view.unmount();
});

it("does not name a left-out container serving a label route", async () => {
  setAppLocale("en-US");
  const { RouteList } = await import("@/features/proxy/RouteList");
  const { instance, route } = await import("@/features/proxy/proxy.test-support");
  let hidden = false;
  function Probe() { hidden = useHiddenUnraidIds().has("c-duplicati"); return null; }
  const routes = [{ ...route, class: "label-read-only", id: "docker:c-duplicati:app", containerName: "duplicati" }] as Parameters<typeof RouteList>[0]["routes"];
  const view = renderWithApp(<><Probe /><RouteList instance={{ ...instance, environmentId: "atlas" }} routes={routes} writable={false} edit={() => {}} action={() => {}} /></>);
  await waitFor(() => expect(view.container.textContent?.includes("duplicati")).toBe(true));
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(hidden).toBe(true));
  await waitFor(() => expect(view.container.textContent?.includes("duplicati")).toBe(false));
  expect(view.container.textContent?.includes("1 hidden")).toBe(true);
  view.unmount();
});
