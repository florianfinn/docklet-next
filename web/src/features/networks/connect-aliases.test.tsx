// Source: docklet-next/issues/204
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original alias input, review and command tests.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { TOPOLOGY } from "@/data/demo/fixtures/topology";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import { ConnectNetworkDialog } from "./ConnectNetworkDialog";

afterEach(() => { simulationStore.reset(); vi.restoreAllMocks(); });

it("validates comma-separated aliases, deduplicates them in review and submits the normalized list", async () => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const adapter = createDemoAdapter();
  const connect = vi.spyOn(adapter, "connectNetwork");
  const close = vi.fn();
  renderWithApp(<ConnectNetworkDialog request={{ mode: "connect", containerId: "c-uptime-kuma", networkId: "n-boreal-shared" }} onClose={close} host={HOSTS.find((h) => h.id === "boreal")!} topology={TOPOLOGY} containers={STANDALONE} />, adapter);
  const dialog = await screen.findByRole("dialog");
  const input = within(dialog).getByLabelText("DNS-Aliasse (optional)");
  await userEvent.type(input, "bad_alias");
  expect((within(dialog).getByRole("button", { name: "Prüfen" }) as HTMLButtonElement).disabled).toBe(true);
  expect(input.getAttribute("aria-invalid")).toBe("true");
  await userEvent.clear(input);
  await userEvent.type(input, "api,");
  expect((within(dialog).getByRole("button", { name: "Prüfen" }) as HTMLButtonElement).disabled).toBe(true);
  await userEvent.clear(input);
  await userEvent.type(input, " API ,api,worker-2");
  await userEvent.click(within(dialog).getByRole("button", { name: "Prüfen" }));
  expect(within(dialog).getByText("api, worker-2") !== null).toBe(true);
  await userEvent.click(within(dialog).getByRole("button", { name: "Zurück" }));
  expect((within(dialog).getByLabelText("DNS-Aliasse (optional)") as HTMLInputElement).value).toBe(" API ,api,worker-2");
  await userEvent.click(within(dialog).getByRole("button", { name: "Prüfen" }));
  await userEvent.click(within(dialog).getByRole("button", { name: "Verbinden" }));
  expect(connect).toHaveBeenCalledWith(expect.objectContaining({ aliases: ["api", "worker-2"] }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
});
