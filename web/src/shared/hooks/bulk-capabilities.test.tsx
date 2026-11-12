// Source: docklet-next/issues/349
// Revision: 199e12dc93dcb6cc44dd708ac816481c81b6503f
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bulk capability reason test (M19 review S4R1-7).

import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";
import { catalogs } from "@/app/i18n/catalog";
import { useBulkCapabilities } from "./use-bulk-run";
import type { BulkItem } from "./bulk-run";

beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());

const items: BulkItem[] = ["a", "b"].map((id) => ({ id: `0~${id}`, name: id, host: "Local", kind: "container" }));
function Probe() { const c = useBulkCapabilities(items, "start"); return <p data-testid="reason">{`${c.available}|${c.reason ?? "none"}`}</p>; }

it.each([
  [["0~a", "0~b"], "true|none"],
  [["0~a"], "true|1 ausgewählter Eintrag wird übersprungen"],
  [[], `false|${catalogs.de.containers.operationReasons.accessFullRequired}`],
])("bulk button reason with allowed %j", async (allowed, text) => {
  const adapter = createDemoAdapter();
  adapter.getCapability = async (operationId, resourceRef = { kind: "hub", id: "0" }) => allowed.includes(resourceRef.id) ? { operationId, resourceRef, available: true } : { operationId, resourceRef, available: false, disabledReasonCode: "access.full-required" };
  renderWithApp(<Probe />, adapter);
  const probe = await screen.findByTestId("reason");
  await waitFor(() => expect(probe.textContent).toBe(text));
});
