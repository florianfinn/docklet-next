// Source: docklet-next/issues/581
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original per-host Unraid include proofs for M37 #581.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import ContainersPage from "@/features/containers/ContainersPage";
import { useVisibleContainers } from "@/shared/hooks/use-host-scope";
import { decodePreferences, DEFAULT_PREFERENCES, preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";

import HostsPage from "./HostsPage";
import { useHostCommands } from "./use-host-commands";

afterEach(() => { preferencesStore.reset(); localStorage.clear(); setAppLocale("de"); });

it("decodes the remembered hosts totally and ignores the old dashboard flag", () => {
  expect(decodePreferences({ showUnraidRing: true }).unraidHidden).toEqual([]);
  expect(decodePreferences({ unraidHidden: "atlas" }).unraidHidden).toEqual([]);
  expect(decodePreferences({ unraidHidden: ["a", "a", 3, null, "", "b".repeat(129), "c"] }).unraidHidden).toEqual(["a", "c"]);
  expect(decodePreferences({ unraidHidden: Array.from({ length: 80 }, (_, i) => `h${i}`) }).unraidHidden.length).toBe(64);
  expect("showUnraidRing" in DEFAULT_PREFERENCES).toBe(false);
});

it("shows the switch only on hosts with Unraid containers and stores the choice per host", async () => {
  setAppLocale("en-US");
  const view = renderWithApp(<HostsPage />);
  await waitFor(() => expect(view.container.querySelector('[data-host="atlas"] [data-slot="unraid-switch"]') !== null).toBe(true));
  const cards = [...view.container.querySelectorAll("[data-host]")];
  expect(cards.length > 1).toBe(true);
  expect(cards.filter((card) => card.querySelector('[data-slot="unraid-switch"]')).map((card) => card.getAttribute("data-host"))).toEqual(["atlas"]);
  const toggle = view.container.querySelector<HTMLElement>('[data-host="atlas"] [role="switch"]')!;
  expect(toggle.getAttribute("aria-checked")).toBe("true");
  await userEvent.click(toggle);
  expect(preferencesStore.get().unraidHidden).toEqual(["atlas"]);
  await userEvent.click(toggle);
  expect(preferencesStore.get().unraidHidden).toEqual([]);
});

it("leaves exactly that host's Unraid containers out of the container list and explains the count", async () => {
  setAppLocale("en-US");
  let seen: { visible: string[]; hidden: number } = { visible: [], hidden: 0 };
  function Probe() { const c = useVisibleContainers(); seen = { visible: c.visible.map((x) => x.name), hidden: c.hiddenUnraid }; return null; }
  renderWithApp(<><Probe /><ContainersPage /></>);
  await waitFor(() => expect(seen.visible.includes("duplicati") && seen.visible.includes("krusader")).toBe(true));
  const before = seen.visible.length;
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(seen.hidden).toBe(2));
  expect(seen.visible.includes("duplicati") || seen.visible.includes("krusader")).toBe(false);
  expect(seen.visible.length).toBe(before - 2);
  expect(await screen.findByText("2 Unraid containers hidden") !== null).toBe(true);
  expect(screen.queryAllByText("krusader").length).toBe(0);
  // Another host in the list does not carry these containers: hiding it changes nothing.
  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["orion"] })));
  await waitFor(() => expect(seen.hidden).toBe(0));
});

it("drops a remembered choice only when the host removal is confirmed", async () => {
  const adapter = createDemoAdapter();
  preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas", "orion"] }));
  let commands: ReturnType<typeof useHostCommands> | undefined;
  function Probe() { commands = useHostCommands(); return null; }
  renderWithApp(<Probe />, { ...adapter, removeHost: async () => { throw new Error("refused"); } });
  await act(async () => { await commands!.remove("atlas"); });
  expect(preferencesStore.get().unraidHidden).toEqual(["atlas", "orion"]);
  const ok = renderWithApp(<Probe />, { ...adapter, removeHost: async () => {} });
  await act(async () => { await commands!.remove("atlas"); });
  expect(preferencesStore.get().unraidHidden).toEqual(["orion"]);
  ok.unmount();
});

it("hides left-out Unraid references in image, volume and network displays without making anything look unused", async () => {
  setAppLocale("en-US");
  const { ConsumerLinks } = await import("@/features/images/ImageRow");
  const { ConsumerChips } = await import("@/features/volumes/storage-visuals");
  const { NetworkTable } = await import("@/features/networks/NetworkTable");
  const { TOPOLOGY } = await import("@/data/demo/fixtures/topology");
  const { HOSTS } = await import("@/data/demo/fixtures/hosts");
  const image = { id: "atlas~img", consumers: [{ containerId: "c-duplicati", name: "duplicati", hostId: "atlas", state: "running" }, { containerId: "c-plain", name: "plain-app", hostId: "atlas", state: "running" }] } as unknown as import("@/domain/images").DockerImage;
  const only = { ...image, consumers: [image.consumers[0]!] } as typeof image;
  const volume = { consumers: [{ containerId: "c-krusader", containerName: "krusader", target: "/data", readOnly: false, running: true }] } as unknown as import("@/domain/volumes").Volume;
  const atlas = HOSTS.find((h) => h.id === "atlas")!;
  const br0 = TOPOLOGY.networks.find((n) => n.id === "n-atlas-br0")!;
  const view = renderWithApp(<>
    <div data-testid="image"><ConsumerLinks image={image} /></div>
    <div data-testid="only"><ConsumerLinks image={only} /></div>
    <div data-testid="volume"><ConsumerChips volume={volume} /></div>
    <NetworkTable host={atlas} networks={[br0]} topology={TOPOLOGY} containers={[]} focus={null} query="" onClearQuery={() => {}} onFocus={() => {}} onConnect={() => {}} onRemove={() => {}} />
  </>);
  const text = (id: string) => view.getByTestId(id).textContent ?? "";
  const usage = () => view.container.querySelector<HTMLElement>('button[aria-label^="br0:"]')?.getAttribute("aria-label") ?? "";
  await waitFor(() => expect(usage().includes("1 container")).toBe(true));
  expect(text("image").includes("duplicati")).toBe(true);
  expect(text("volume").includes("krusader")).toBe(true);

  act(() => preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas"] })));
  await waitFor(() => expect(text("image").includes("duplicati")).toBe(false));
  expect(text("image").includes("plain-app")).toBe(true);
  expect(text("only")).toBe("1 hidden");
  expect(text("volume")).toBe("1 hidden");
  expect(usage()).toBe("br0: In use, 0 containers");
});

it("keeps a detected Unraid switch while the inventory is unknown and drops it only on a fresh empty result, a new sign-in or the host's removal", async () => {
  const { unraidDetected, resetUnraidDetection, forgetUnraidDetection } = await import("./unraid-detection");
  resetUnraidDetection();
  expect(unraidDetected(1, "atlas", undefined)).toBe(false);
  expect(unraidDetected(1, "atlas", 2)).toBe(true);
  expect(unraidDetected(1, "atlas", undefined)).toBe(true);
  expect(unraidDetected(1, "atlas", undefined)).toBe(true);
  expect(unraidDetected(1, "atlas", 0)).toBe(false);
  expect(unraidDetected(1, "atlas", undefined)).toBe(false);
  // Another sign-in starts without the previous user's detections.
  expect(unraidDetected(1, "atlas", 1)).toBe(true);
  expect(unraidDetected(2, "atlas", undefined)).toBe(false);
  // A removed host's ID comes back undetected.
  expect(unraidDetected(2, "atlas", 1)).toBe(true);
  forgetUnraidDetection("atlas");
  expect(unraidDetected(2, "atlas", undefined)).toBe(false);
  resetUnraidDetection();
});

it("never prunes a remembered choice from a failed or partial host list", async () => {
  setAppLocale("en-US");
  preferencesStore.set((p) => ({ ...p, unraidHidden: ["atlas", "gone"] }));
  const adapter = createDemoAdapter();
  const partial = renderWithApp(<HostsPage />, { ...adapter, listHosts: async () => (await adapter.listHosts()).filter((h) => h.id !== "atlas") });
  await waitFor(() => expect(partial.container.querySelectorAll("[data-host]").length > 0).toBe(true));
  expect(partial.container.querySelector('[data-host="atlas"]') === null).toBe(true);
  expect(preferencesStore.get().unraidHidden).toEqual(["atlas", "gone"]);
  partial.unmount();
  const failed = renderWithApp(<HostsPage />, { ...adapter, listHosts: async () => { throw new Error("read failed"); } });
  // The failure has settled once the page offers its retry.
  expect(await screen.findByRole("button", { name: "Try again" }) !== null).toBe(true);
  expect(failed.container.querySelectorAll("[data-host]").length).toBe(0);
  expect(preferencesStore.get().unraidHidden).toEqual(["atlas", "gone"]);
});
