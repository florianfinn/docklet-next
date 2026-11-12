// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W2 producer-to-query-to-control capability refresh regression.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { useContainers } from "@/data/queries";
import { useCapability } from "@/data/use-capability";
import { renderWithApp } from "@/test/render";
import { ContainerProvenance } from "./ContainerProvenance";

beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());
function CurrentOrigin() {
  const containers = useContainers();
  const capability = useCapability("recreateContainer", { kind: "container", id: "c-netdata" });
  const c = containers.data?.find((item) => item.id === "c-netdata");
  return c ? <><ContainerProvenance c={c} /><output>{c.origin}</output><button disabled={!capability.available}>Edit</button></> : null;
}

it("W2 refreshes the actual edit control after assignment and locks it again after detection reset", async () => {
  renderWithApp(<CurrentOrigin />, createDemoAdapter());
  await screen.findByText("unknown", { selector: "output" });
  await waitFor(() => expect((screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement).disabled).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Herkunft zuweisen" }));
  let dialog = await screen.findByRole("dialog");
  within(dialog).getByRole("combobox").focus(); await userEvent.keyboard("{ArrowDown}");
  await userEvent.click(await screen.findByRole("option", { name: "Run" }));
  await userEvent.click(within(dialog).getByRole("button", { name: "Zuweisen" }));
  await screen.findByText("standalone", { selector: "output" });
  await waitFor(() => expect((screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(screen.getByRole("button", { name: "Herkunft zuweisen" }));
  dialog = await screen.findByRole("dialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Auf Erkennung zurücksetzen" }));
  await screen.findByText("unknown", { selector: "output" });
  await waitFor(() => expect((screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement).disabled).toBe(true));
});
