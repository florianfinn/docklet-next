// Source: docklet-next/issues/626
// Revision: 505d1715fdf1dc1668655e9affef78b67620f7d9
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original target snapshots, current inventory links and status regression coverage.

import { useQueryClient } from "@tanstack/react-query";
import { useContainers, useStacks } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { catalogs } from "@/app/i18n/catalog";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";
import { JobUpdateOutcomes } from "./job-update-outcomes";

const at = "2026-11-25T12:00:00Z";
const targets = [
  { id: "direct~stack~project", environmentId: "direct", targetKind: "stack" as const, targetId: "project", name: "Example stack", status: "succeeded" as const, updateAvailable: true },
  { id: "direct~container~app", environmentId: "direct", targetKind: "container" as const, targetId: "app", name: "Example container", status: "failed" as const, code: "update.check-unavailable" },
  { id: "direct~stack~removed", environmentId: "direct", targetKind: "stack" as const, targetId: "removed", name: "Removed stack", status: "skipped" as const, code: "update.image-not-pulled" },
];
const run = renderRun(mapActivityDetail({ activity: { id: "example", environmentId: "0", type: "job_run", status: "failed", startedAt: at, createdAt: at, metadata: { jobId: "update-check-host", runId: "example-run" } }, stepTrace: [], messages: [], output: [], outputTruncated: false }), "en-US");
async function fixture(stale = false, containerName = "app", containerHost = "direct", containerId = "replacement") {
  const adapter = createDemoAdapter();
  const stack = (await adapter.listStacks())[0]!, container = (await adapter.listContainers())[0]!;
  adapter.listStacks = async () => [{ ...stack, id: "direct~project", hostId: "direct", name: "Current stack", inventory: { stale } }];
  adapter.listContainers = async () => [{ ...container, id: `${containerHost}~${containerId}`, hostId: containerHost, name: containerName, inventory: { stale } }];
  adapter.getUpdateJobTargets = async () => targets;
  return adapter;
}
function InventoryReady() {
  const stacks = useStacks(), containers = useContainers();
  return <span data-testid="inventory-ready">{String(stacks.isSuccess && containers.isSuccess)}</span>;
}
afterEach(() => { cleanup(); setAppLocale("de"); });
it("decodes the producer's name, update availability and current-attempt membership", async () => {
  const f = updateFixture(), original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => String(input).includes("/jobs/update-check-host/runs/example-run") ? Response.json({ id: "example-run", jobId: "update-check-host", environmentId: "0", outcome: { plannedIds: [targets[0]!.id], plannedKnown: true, targets } }) : original(input, init));
  await f.adapter.getCurrentUser();
  const decoded = await f.adapter.getUpdateJobTargets!("0", "update-check-host", "example-run");
  expect(decoded[0]).toMatchObject({ name: "Example stack", updateAvailable: true, inCurrentPlan: true });
  expect(decoded[1]?.inCurrentPlan).toBe(false);
});
it("navigates a present stack by a real click and shows names, status tones and localized skip reason", async () => {
  setAppLocale("en-US"); const view = renderWithApp(<><InventoryReady /><JobUpdateOutcomes run={run} /></>, await fixture());
  await waitFor(() => expect(view.container.querySelector("[data-job-target]") !== null).toBe(true));
  await waitFor(() => expect(screen.getByTestId("inventory-ready").textContent).toBe("true"));
  expect(view.container.textContent).toContain("Example stack");
  expect(screen.queryByRole("link", { name: "Example stack" }) !== null).toBe(true);
  expect(view.container.querySelector('[data-job-target="direct~stack~project"] [data-tone="ok"]') !== null).toBe(true);
  expect(view.container.querySelector('[data-job-target="direct~container~app"] [data-tone="err"]') !== null).toBe(true);
  expect(view.container.querySelector('[data-job-target="direct~stack~removed"] [data-tone="warn"]') !== null).toBe(true);
  expect(screen.getByText("Update available").getAttribute("data-tone")).toBe("info");
  expect(screen.getByText("Removed stack").closest("a") === null).toBe(true);
  expect(view.container.textContent).toContain(catalogs["en-US"].images.errors["update_image-not-pulled"]);
  await userEvent.click(await screen.findByRole("link", { name: "Example stack" }));
  expect(view.router.state.location.pathname).toBe("/stacks/direct~project");
});
it.each(["original", "replacement"])("links a %s container by its stable name", async (id) => {
  setAppLocale("en-US"); const view = renderWithApp(<><InventoryReady /><JobUpdateOutcomes run={run} /></>, await fixture(false, "app", "direct", id));
  await waitFor(() => expect(screen.getByTestId("inventory-ready").textContent).toBe("true"));
  expect(screen.queryByRole("link", { name: "Example container" }) !== null).toBe(true);
  const link = screen.getByRole("link", { name: "Example container" });
  await userEvent.click(link);
  expect(view.router.state.location.pathname).toBe("/containers/direct~app");
});
it.each([[true, "app", "direct"], [false, "renamed", "direct"], [false, "app", "other"]] as const)("suppresses stale, renamed or cross-host target links (%s, %s, %s)", async (stale, name, host) => {
  const view = renderWithApp(<><InventoryReady /><JobUpdateOutcomes run={run} /></>, await fixture(stale, name, host));
  await waitFor(() => expect(screen.getByTestId("inventory-ready").textContent).toBe("true"));
  const label = await screen.findByText("Example container");
  await waitFor(() => expect(label.closest("a") === null).toBe(true));
  if (stale) expect(screen.getByText("Example stack").closest("a") === null).toBe(true);
  expect(view.container.textContent).toContain("Removed stack");
});

it("keeps prior-attempt rows visible with an explicit exclusion label and legacy fallback IDs in monospace", async () => {
  setAppLocale("en-US"); const adapter = await fixture();
  adapter.getUpdateJobTargets = async () => [{ ...targets[0]!, name: undefined, inCurrentPlan: false }];
  const view = renderWithApp(<JobUpdateOutcomes run={run} />, adapter);
  await screen.findByText("Not part of this check");
  const label = screen.getByText("project");
  // A legacy ID keeps its code styling even when it resolves to a link.
  expect(label.classList.contains("font-mono")).toBe(true);
  expect(view.container.querySelector("[data-job-target]") !== null).toBe(true);
});

it("removes a retained stack link when its inventory reread becomes unavailable", async () => {
  setAppLocale("en-US"); const adapter = await fixture();
  function InvalidateInventory() {
    const client = useQueryClient();
    return <button onClick={() => { adapter.listStacks = async () => { throw new Error("Unavailable example inventory"); }; void client.invalidateQueries({ queryKey: queryKeys.stacks }); }}>Refresh inventory</button>;
  }
  renderWithApp(<><InvalidateInventory /><JobUpdateOutcomes run={run} /></>, adapter);
  await screen.findByRole("link", { name: "Example stack" });
  await userEvent.click(screen.getByRole("button", { name: "Refresh inventory" }));
  await waitFor(() => expect(screen.getByText("Example stack").closest("a") === null).toBe(true));
});

it("renders a malformed target snapshot as an unlinked row instead of failing the list", async () => {
  setAppLocale("en-US"); const adapter = await fixture();
  adapter.getUpdateJobTargets = async () => [{ ...targets[0]!, targetId: "project~extra", name: "Malformed stack" }, targets[1]!];
  renderWithApp(<><InventoryReady /><JobUpdateOutcomes run={run} /></>, adapter);
  await screen.findByText("Malformed stack");
  await waitFor(() => expect(screen.getByTestId("inventory-ready").textContent).toBe("true"));
  expect(screen.getByText("Malformed stack").closest("a") === null).toBe(true);
  expect(screen.queryByText("Example container") !== null).toBe(true);
});
