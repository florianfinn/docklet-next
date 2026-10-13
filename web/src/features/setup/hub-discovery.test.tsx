import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { DISCOVERY_BRIDGE, DISCOVERY_BRIDGE_UNKNOWN, DISCOVERY_HOST, DISCOVERY_MACVLAN } from "@/data/demo/fixtures/hub-discovery";
import { simulationStore } from "@/data/demo/simulation";
import { hubAddressCandidates, type HubDiscovery } from "@/domain/hub-discovery";
import { renderRoutes } from "@/test/render";

import { applyDiscovery, freshSetupDraft, restartSetup, setupStore } from "./setup-draft";
import SetupPage from "./SetupPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  restartSetup();
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const best = (d: HubDiscovery) => hubAddressCandidates(d).find((c) => c.recommended)?.address;
const field = () => screen.getByLabelText("Interne Hub-Adresse") as HTMLInputElement;

describe("hub address candidates", () => {
  it("prefers LAN interfaces, then host addresses, and never recommends a bridge address", () => {
    expect(hubAddressCandidates(DISCOVERY_MACVLAN).map((c) => [c.address, c.reach, c.recommended])).toEqual([
      ["192.0.2.10:8080", "lan", true],
      ["docklet.example.com", "unknown", false],
      ["198.51.100.226:8080", "same-host", false],
    ]);
    // Bridge: the configured host address with the published port; the request host is already covered.
    expect(hubAddressCandidates(DISCOVERY_BRIDGE).map((c) => c.address)).toEqual(["192.0.2.10:8080", "198.51.100.242:8080"]);
    // Host network: loopback and link-local interfaces are dropped, so is a localhost request.
    expect(hubAddressCandidates(DISCOVERY_HOST).map((c) => c.address)).toEqual(["192.0.2.10:8080"]);
    expect(best(DISCOVERY_BRIDGE_UNKNOWN)).toBe("docklet-hub.example.net:8080");
    expect(best({ ...DISCOVERY_BRIDGE_UNKNOWN, requestHost: undefined })).toBe(undefined);
  });

  it("fills an empty untouched field only", () => {
    const filled = applyDiscovery(freshSetupDraft(), DISCOVERY_MACVLAN);
    expect([filled.general.hubAddress, filled.hubAddressOrigin]).toEqual(["192.0.2.10:8080", { kind: "discovered", candidateId: "if:br0" }]);
    const typed = { ...freshSetupDraft(), hubAddressOrigin: { kind: "manual" as const } };
    expect(applyDiscovery(typed, DISCOVERY_MACVLAN)).toBe(typed);
    const cleared = { ...freshSetupDraft(), general: { ...freshSetupDraft().general, hubAddress: "" }, hubAddressOrigin: { kind: "manual" as const } };
    expect(applyDiscovery(cleared, DISCOVERY_MACVLAN).general.hubAddress).toBe("");
  });
});

describe("setup prefill", () => {
  it("keeps what the user typed when discovery answers late and offers the proposal explicitly", async () => {
    let answer: (d: HubDiscovery) => void = () => {};
    const adapter = { ...createDemoAdapter(), discoverHub: () => new Promise<HubDiscovery>((r) => (answer = r)) };
    renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"], adapter);
    expect(await screen.findByText("Hub erkennt seine Netzwerke …") !== null).toBe(true);
    await userEvent.type(field(), "hub.example.org:8080");
    answer(DISCOVERY_BRIDGE);
    const list = await screen.findByRole("list", { name: "Erkannte Adressen" });
    expect(field().value).toBe("hub.example.org:8080");
    expect(screen.getByText(/Manuell eingetragen/) !== null).toBe(true);

    await userEvent.click(within(list).getByRole("button", { name: "192.0.2.10:8080 übernehmen" }));
    expect(field().value).toBe("192.0.2.10:8080");
    expect(screen.getByText(/Vorausgefüllt mit 192\.0\.2\.10:8080 \(Host-Adresse\)/) !== null).toBe(true);
    expect(within(list).getByText("nur auf diesem Host") !== null).toBe(true);
  });

  it("prefills again after a replay", async () => {
    renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"]);
    await waitFor(() => expect(field().value).toBe("192.0.2.10:8080"));
    setupStore.set((d) => ({ ...d, general: { ...d.general, hubAddress: "x" }, hubAddressOrigin: { kind: "manual" } }));
    restartSetup();
    await waitFor(() => expect(field().value).toBe("192.0.2.10:8080"));
  });

  it("falls back to manual entry when discovery fails", async () => {
    const adapter = { ...createDemoAdapter(), discoverHub: () => Promise.reject(new Error("boom")) };
    renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"], adapter);
    expect((await screen.findByRole("alert")).textContent?.includes("Adresse bitte selbst eintragen")).toBe(true);
    expect(field().value).toBe("");
  });
});
