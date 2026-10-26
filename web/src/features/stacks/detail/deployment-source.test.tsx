// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231.

import { useState } from "react";
import { useActionsQuery } from "@/data/queries";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { activity, activityFixture } from "@/data/http/activity-reader.test-support";
import type { StackDetail } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import type { Detail } from "@/data/http/activity-reader.test-support";
import { DeploymentsTab } from "./DeploymentsTab";

const stack: StackDetail = { id: "good~project", hostId: "good", name: "Example", labels: [], services: [], autoUpdate: { mode: "check" }, deployments: [], restorePoints: [], compose: "", env: "" };
it.each([false, true])("shows editor, update and restore runs from HTTP summaries (Edge=%s)", async (edge) => {
  const f = activityFixture(edge);
  f.state.hosts = ["good"];
  f.state.lists.good = ["project_deploy", "project_update", "project_restore"].map((type, index) => ({ ...activity("good", `deployment-${index}`), type, status: "success", resourceType: "stack", resourceId: "project", startedAt: `2026-10-26T0${index}:00:00Z`, endedAt: `2026-10-26T0${index}:01:00Z` }));
  for (const row of f.state.lists.good) f.state.details[row.id] = { activity: row, messages: [], output: [], stepTrace: [], outputTruncated: false };
  await f.adapter.getCurrentUser();
  renderWithApp(<DeploymentsTab stack={stack} />, f.adapter);
  await act(async () => { await f.adapter.listActions(); });
  expect(document.querySelector('[data-slot="deployment-current"]')?.getAttribute("data-run")).toBe("deployment-2");
  expect(screen.queryByText(catalogs.de.stacks.deployments.empty) === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: /2 frühere/ }));
  await userEvent.click(screen.getByRole("button", { name: `${catalogs.de.activity.runKind.update} anzeigen` }));
  await waitFor(() => expect(document.querySelector('[data-slot="deployment-current"]')?.getAttribute("data-run")).toBe("deployment-1"));
  await userEvent.click(screen.getByRole("button", { name: `${catalogs.de.activity.runKind.deploy} anzeigen` }));
  await waitFor(() => expect(document.querySelector('[data-slot="deployment-current"]')?.getAttribute("data-run")).toBe("deployment-0"));
  expect(f.requests.some((path) => path.includes("/updater/history"))).toBe(false);
  expect(f.requests.filter((path) => /\/activities\?/.test(path)).every((path) => path.includes("limit=-1"))).toBe(true);
});
it("uses neutral unavailable history after source failure and recovers on a fresh list", async () => {
  const f = activityFixture(); f.state.hosts = ["bad"]; f.state.failure = "page502";
  renderWithApp(<DeploymentsTab stack={{ ...stack, id: "bad~project", hostId: "bad" }} />, f.adapter);
  await waitFor(() => expect(f.requests.some((path) => path.includes("/bad/activities?"))).toBe(true));
  expect(screen.queryByText(catalogs.de.stacks.deployments.empty) === null).toBe(true);
  expect(screen.queryByText(catalogs.de.stacks.deployments.unavailable) !== null).toBe(true);
  f.state.failure = ""; f.state.lists.bad = [];
  await act(async () => { await f.adapter.listActions(); });
  await screen.findByText(catalogs.de.stacks.deployments.empty);
});

it("opening Deployments reuses an existing shared discovery window", async () => {
  const f = activityFixture(); f.state.hosts = ["good"]; f.state.lists.good = [];
  await f.adapter.getCurrentUser();
  function SharedSource() { const source = useActionsQuery(), [open, setOpen] = useState(false); return <><button disabled={!source.isSuccess} onClick={() => setOpen(true)}>Open deployments</button>{open && <DeploymentsTab stack={stack} />}</>; }
  renderWithApp(<SharedSource />, f.adapter);
  const open = screen.getByRole("button", { name: "Open deployments" });
  await waitFor(() => expect(open.hasAttribute("disabled")).toBe(false));
  const count = f.requests.filter((path) => /\/activities\?/.test(path)).length;
  await userEvent.click(open); await screen.findByText(catalogs.de.stacks.deployments.empty);
  expect(f.requests.filter((path) => /\/activities\?/.test(path))).toHaveLength(count);
});


const progressCases = [
  { name: "plain deploy", type: "project_deploy", lifecycle: false, failed: false },
  { name: "deploy with imageLifecycle", type: "project_deploy", lifecycle: true, failed: false },
  { name: "project_update", type: "project_update", lifecycle: true, failed: false },
  { name: "restore", type: "project_restore", lifecycle: true, failed: false },
  { name: "failed unknown error", type: "project_update", lifecycle: true, failed: true },
];
for (const edge of [false, true]) it.each(progressCases)(`preserves $name progress in RunCard and RunFocus (Edge=${edge})`, async ({ type, lifecycle, failed }) => {
  const f = activityFixture(edge); f.state.hosts = ["good"];
  const row = { ...activity("good", "progress-run"), type, resourceType: "stack", resourceId: "project", status: failed ? "failed" : "running", latestMessage: "Deploying project", ...(failed ? { error: "private raw diagnostic" } : {}), ...(lifecycle ? { imageLifecycle: { runId: "progress-run", verified: !failed, volumeDataIncluded: false as const, ...(failed ? { code: "unknown.error-code" } : {}) } } : {}) };
  const detail: Detail = { activity: row, stepTrace: [{ id: "progress-step", name: "Deploying project", status: failed ? "failed" : "running", startedAt: row.startedAt }], messages: [{ id: "progress-message", activityId: row.id, level: "info", message: "Pulled image", createdAt: row.startedAt }], output: [], outputTruncated: false };
  const mapped = mapActivityDetail(detail), rendered = renderRun(mapped, "de");
  expect(rendered.steps[0]!.label).toBe("Deploying project"); expect(rendered.log).toEqual(["Pulled image"]);
  const latest = renderRun(mapActivityDetail({ ...detail, messages: [] }), "de"); expect(latest.log).toEqual(["Deploying project"]);
  const unknown = renderRun(mapActivityDetail({ ...detail, messages: [{ ...detail.messages![0]!, code: "progress.future" as never, message: "Waiting for health" }] }), "de"); expect(unknown.log).toEqual(["Waiting for health"]);
  if (lifecycle) {
    const known = renderRun(mapActivityDetail({ ...detail, messages: [{ ...detail.messages![0]!, code: "update.config-drift" as never }] }), "de"); expect(known.log).toEqual([catalogs.de.images.errors["update_config-drift"]]);
    const started = mapActivityDetail({ ...detail, messages: [{ ...detail.messages![0]!, code: "run.started" }] }); expect(started.log[0]!.code).toBe("activity.status.running");
  }
  if (failed) { expect(rendered.error).toBe(catalogs.de.common.commandErrors.failed); expect(JSON.stringify(mapped)).not.toContain("private raw diagnostic"); }
  f.state.lists.good = [row]; f.state.details[row.id] = detail;
  await f.adapter.getCurrentUser(); renderWithApp(<DeploymentsTab stack={stack} />, f.adapter);
  await act(async () => { await f.adapter.listActions(); });
  await waitFor(() => expect(document.querySelector('[data-slot="run-focus"]')?.textContent).toContain("Deploying project"));
  const card = document.querySelector('[data-slot="deployment-current"]')!;
  await userEvent.click(within(card as HTMLElement).getByRole("button", { name: catalogs.de.stacks.deployments.steps }));
  expect(within(card as HTMLElement).getAllByText("Deploying project").length > 0).toBe(true);
  expect(card.querySelector('[data-slot="run-focus"]')?.textContent).toContain("Deploying project");
  expect(f.requests.some((path) => path.includes("/activities/progress-run?"))).toBe(true);
  if (failed) expect(card.textContent).toContain(catalogs.de.common.commandErrors.failed);
  else expect(card.textContent).not.toContain(catalogs.de.common.commandErrors.failed);
});

it("warns about a failed stack host while preserving its cached deployments", async () => {
  const f = activityFixture(); f.state.hosts = ["bad"];
  const row = { ...activity("bad"), type: "project_deploy", status: "success", resourceType: "stack", resourceId: "project" };
  f.state.lists.bad = [row]; f.state.details[row.id] = { activity: row, stepTrace: [], messages: [], output: [], outputTruncated: false };
  await f.adapter.getCurrentUser(); renderWithApp(<DeploymentsTab stack={{ ...stack, id: "bad~project", hostId: "bad" }} />, f.adapter);
  await act(async () => { await f.adapter.listActions(); });
  await waitFor(() => expect(document.querySelector('[data-slot="deployment-current"]')?.getAttribute("data-run")).toBe(row.id));
  f.state.failure = "page502";
  await act(async () => { await f.adapter.listActions(); });
  expect(screen.queryByText(catalogs.de.stacks.deployments.unavailable) !== null).toBe(true);
  expect(document.querySelector('[data-slot="deployment-current"]')?.getAttribute("data-run")).toBe(row.id);
});
