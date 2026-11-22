// Source: docklet-next/issues/568
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original no-proof, drift and confirmation gestures through the ordinary deploy adapter.; M36 #568: record baseline without pulling images.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { useQuery } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderStackDetail } from "@/data/message-code";
import type { UpdateBaseline } from "@/domain/stack-updates";
import { renderWithApp } from "@/test/render";
import { RecordBaseline } from "./RecordBaseline";

const baseline: UpdateBaseline = { targetIdentity: "example", savedRevision: "", observedRevision: "", available: false, autoExcluded: false, code: "update.baseline-unavailable" };
afterEach(() => { cleanup(); setAppLocale("de"); });

function ReadObservation({ stackId }: { stackId: string }) {
  const result = useQuery({ queryKey: ["stack", stackId, "update-baseline"], queryFn: async () => baseline, enabled: false });
  return result.data ? <span>Baseline observed</span> : null;
}

async function fixture(value: UpdateBaseline = baseline, disabled = false, state?: "stopped") {
  setAppLocale("en-US");
  const adapter = createDemoAdapter();
  const read = renderStackDetail(await adapter.getStack("media"), "en-US");
  const stack = { ...read, services: read.services.map((service, index) => ({ ...service, state: index === 0 && state ? state : "running" as const })) };
  adapter.getUpdateBaseline = vi.fn(async () => value);
  const original = adapter.getCapability;
  vi.spyOn(adapter, "getCapability").mockImplementation((operation, ref) => operation === "getUpdateBaseline" ? Promise.resolve({ operationId: operation, resourceRef: ref ?? { kind: "hub", id: "0" }, available: true }) : original(operation, ref));
  const deploy = vi.spyOn(adapter, "runStackCommand").mockResolvedValue("baseline-run");
  renderWithApp(<><RecordBaseline stack={stack} host={undefined} inline={false} disabled={disabled} /><ReadObservation stackId={stack.id} /></>, adapter);
  await screen.findByText("Baseline observed");
  return { adapter, stack, deploy };
}

it("offers a no-proof baseline, explains saved edits and admits a normal deploy with pull never", async () => {
  const f = await fixture();
  expect(await screen.findByText("No baseline yet") !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Record baseline" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(within(dialog).getByText("Records the current deployment as baseline by running a normal deploy without pulling images. Saved but undeployed changes are applied; unchanged containers are not recreated.") !== null).toBe(true);
  await userEvent.click(within(dialog).getByRole("button", { name: "Record baseline" }));
  await waitFor(() => expect(f.deploy).toHaveBeenCalledExactlyOnceWith(f.stack.id, "start", { pullPolicy: "never" }));
});

it.each([
  { ...baseline, code: "update.config-drift" },
  { ...baseline, sourceRunId: "known-run" },
  { ...baseline, available: true, code: undefined },
])("does not offer baseline recording when the read has proof or reports drift: $code", async (value) => {
  await fixture(value);
  await waitFor(() => expect(screen.queryByRole("button", { name: "Record baseline" }) === null).toBe(true));
  expect(screen.queryByText("No baseline yet") === null).toBe(true);
});

it("keeps the action disabled while the ordinary deploy is unavailable", async () => {
  const f = await fixture(baseline, true);
  const button = await screen.findByRole("button", { name: "Record baseline" });
  expect((button as HTMLButtonElement).disabled).toBe(true);
  await userEvent.click(button);
  expect(f.deploy).not.toHaveBeenCalled();
});

it("offers baseline recording only while every service runs (recording would start stopped services)", async () => {
  await fixture(baseline, false, "stopped");
  await waitFor(() => expect(screen.queryByRole("button", { name: "Record baseline" }) === null).toBe(true));
  expect(screen.queryByText("No baseline yet") === null).toBe(true);
});
