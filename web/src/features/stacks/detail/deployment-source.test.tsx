// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231.

import { useState } from "react";
import { useActionsQuery } from "@/data/queries";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { activity, activityFixture } from "@/data/http/activity-reader.test-support";
import type { StackDetail } from "@/domain/model";
import { renderWithApp } from "@/test/render";
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
  expect(screen.getByText(catalogs.de.stacks.deployments.unavailable) !== null).toBe(true);
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
