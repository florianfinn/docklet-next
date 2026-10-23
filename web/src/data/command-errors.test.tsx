// Source: docklet-next/issues/204
// Revision: d03b817a6aa2286616ba32fc3b04a9d3f82422ed
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original DE consumer tests for typed network/volume outcomes and unknown exception text.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { AdapterError } from "./api";
import { AdapterOutcomeError, unavailable } from "./capability";
import { networkDraft, networkFixture } from "./http/network-commands.test-support";
import { useNetworkCommands } from "@/features/networks/use-network-commands";
import { useVolumeCommands } from "@/features/volumes/use-volume-commands";
import { useImageCommands } from "@/features/images/use-image-commands";
import { renderWithApp } from "@/test/render";

const generic = "Die Anfrage konnte nicht abgeschlossen werden.";
const cases = [
  ["unknown code", () => new AdapterOutcomeError({ kind: "error", code: "network.future-code", status: 400 }), generic],
  ["5xx", () => new AdapterOutcomeError({ kind: "error", code: "http.error", status: 500 }), generic],
  ["agent.unavailable", () => new AdapterOutcomeError({ kind: "error", code: "agent.unavailable", status: 502 }), "Der Host ist nicht erreichbar. Prüfe die Verbindung."],
  ["proxy 504", () => new AdapterOutcomeError({ kind: "error", code: "agent.unavailable", status: 504 }), "Der Host ist nicht erreichbar. Prüfe die Verbindung."],
  ["auth failure", () => new AdapterOutcomeError({ kind: "error", code: "auth.required", status: 401 }), "Für diese Aktion erneut anmelden."],
  ["forbidden", () => new AdapterOutcomeError({ kind: "error", code: "auth.denied", status: 403 }), "Für diese Aktion fehlt die Berechtigung."],
  ["withdrawn capability", () => unavailable("createNetwork", { kind: "host", id: "host" }), "Diese Aktion ist nicht verfügbar, weil ihre Fähigkeit nicht gelesen werden konnte."],
  ["mapping unavailable", () => unavailable("createNetwork", { kind: "host", id: "host" }, "capability.mapping-unavailable"), "Diese Aktion ist in der aktuellen Hub-Version nicht verfügbar."],
  ["English detail", () => new Error("Failed to create resource"), generic],
  ["literal outcome kind", () => new Error("error"), generic],
] as const;
afterEach(() => vi.restoreAllMocks());

// The actual consumer receives these AdapterOutcomeError shapes. Identity invalidation may suppress
// a real 401 earlier; rejection injection keeps that separate from the consumer's text contract.
describe.each(["networks", "volumes", "images"] as const)("%s command error messages", (feature) => {
  it.each(cases)("translates %s without exposing raw exception text", async (_name, failure, expected) => {
    const f = networkFixture(); const error = vi.spyOn(toast, "error"); const settled = vi.fn();
    const reject = vi.fn(async () => { throw failure(); });
    const adapter = { ...f.adapter, createNetwork: reject, createVolume: reject, pullImage: reject };
    function NetworkProbe() { const cmd = useNetworkCommands("host"); return <button onClick={() => void cmd.create(networkDraft()).then(settled)}>Run</button>; }
    function VolumeProbe() { const cmd = useVolumeCommands(); return <button onClick={() => void cmd.create({ hostId: "host", name: "example", labels: {} }).then(settled)}>Run</button>; }
    function ImageProbe() { const cmd = useImageCommands(); return <button onClick={() => void cmd.pull({ hostId: "host", reference: "example:1.0.0" }).then(settled)}>Run</button>; }
    renderWithApp(feature === "networks" ? <NetworkProbe /> : feature === "volumes" ? <VolumeProbe /> : <ImageProbe />, adapter);
    await userEvent.click(screen.getByRole("button", { name: "Run" }));
    await waitFor(() => expect(settled).toHaveBeenCalledOnce());
    expect(error).toHaveBeenCalledWith(expected);
    expect(error).toHaveBeenCalledTimes(1);
  });
});

it.each(["networks", "volumes"] as const)("retains the typed in-use domain message for %s", async (feature) => {
  const f = networkFixture(); const error = vi.spyOn(toast, "error"); const settled = vi.fn();
  const reject = vi.fn(async () => { throw new AdapterError("in-use"); });
  const adapter = { ...f.adapter, createNetwork: reject, createVolume: reject };
  function NetworkProbe() { const cmd = useNetworkCommands("host"); return <button onClick={() => void cmd.create(networkDraft()).then(settled)}>Run</button>; }
  function VolumeProbe() { const cmd = useVolumeCommands(); return <button onClick={() => void cmd.create({ hostId: "host", name: "example", labels: {} }).then(settled)}>Run</button>; }
  renderWithApp(feature === "networks" ? <NetworkProbe /> : <VolumeProbe />, adapter);
  await userEvent.click(screen.getByRole("button", { name: "Run" }));
  await waitFor(() => expect(settled).toHaveBeenCalledOnce());
  expect(error).toHaveBeenCalledWith("Noch in Benutzung.");
  expect(error.mock.calls[0]?.[0]).not.toBe(generic);
});
