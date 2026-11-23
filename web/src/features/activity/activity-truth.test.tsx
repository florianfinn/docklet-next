// Source: docklet-next/issues/584
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, cleanup, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { deferred } from "@/data/http/activity-reader.test-support";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import type { ActionRun, Host } from "@/domain/model";
import { useState } from "react";
import { renderWithApp } from "@/test/render";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";
import { ActionDialog } from "./ActionDialog";
import { actionDialogStore, openActionDialog } from "./action-dialog-store";
import { ActionToastView } from "./ActionToastView";
import { RunFocusBody } from "./RunFocus";
import { RunOutput } from "./RunOutput";

const at = "2026-11-23T10:00:00Z";
const coded = (status = "success", change = {}) => mapActivityDetail({ activity: { id: "example-run", environmentId: "0", resourceType: "system", resourceId: "0", type: "hub.update", metadata: { targetVersion: "1.5.0" }, status, startedAt: at, createdAt: at, ...change }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
afterEach(() => { cleanup(); actionDialogStore.reset(); activityStore.reset(); setAppLocale("de"); });

it.each(["en-US", "de"] as const)("reacts to late hosts, rename and removal in row, dialog, focus and all toast designs (%s)", async (locale) => {
  setAppLocale(locale);
  const adapter = createDemoAdapter(), hosts = deferred<Host[]>(), run = coded();
  adapter.listHosts = () => hosts.promise;
  adapter.listActions = async () => [run];
  adapter.subscribeActions = (listener) => { listener([run]); return () => {}; };
  adapter.observeActionDetail = (_host, _id, listener) => { listener({ status: "ready", run }); return () => {}; };
  let client!: QueryClient;
  function Surfaces() {
    client = useQueryClient();
    const rendered = renderRun(run, locale);
    return <><ActivityCenter /><ActionDialog /><RunFocusBody run={rendered} now={0} />{(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={rendered} design={design} detail="full" now={0} major onClose={() => {}} onOpen={() => {}} />)}</>;
  }
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  const view = renderWithApp(<Surfaces />, adapter);
  await waitFor(() => expect(document.querySelector("#run-example-run") !== null).toBe(true));
  act(() => openActionDialog(run.id));
  await waitFor(() => expect(document.querySelector('[data-layer="action-dialog"]') !== null).toBe(true));
  const targets = () => [...document.querySelectorAll('[data-fact="target"] dd, [data-slot="toast-meta"] > div:first-child dd')].map((e) => e.textContent);
  const description = () => document.querySelector('[data-layer="action-dialog"] [data-slot="dialog-description"]')?.textContent;
  const row = () => document.querySelector("#run-example-run")!.textContent!;
  expect(targets().length).toBeGreaterThanOrEqual(3);
  expect(targets().every((t) => t === "")).toBe(true);
  expect(description()).toBe(locale === "de" ? "Hub-Update auf 1.5.0" : "Hub update to 1.5.0");
  expect(row()).not.toMatch(/ · (?:0|$)/);
  await act(async () => { hosts.resolve([{ id: "0", name: "Example Hub" } as Host]); await hosts.promise; });
  await waitFor(() => expect(targets().every((t) => t === "Example Hub")).toBe(true));
  expect(description()).toContain(" · Example Hub"); expect(row()).toContain(" · Example Hub");
  act(() => client.setQueryData(["hosts"], [{ id: "0", name: "Renamed Hub" }]));
  await waitFor(() => expect(targets().every((t) => t === "Renamed Hub")).toBe(true));
  expect(row()).toContain(" · Renamed Hub"); expect(description()).toContain(" · Renamed Hub");
  act(() => client.setQueryData(["hosts"], []));
  await waitFor(() => expect(targets().every((t) => t === "")).toBe(true));
  expect(description()).not.toContain(" · "); expect(row()).not.toContain(" · Renamed Hub"); expect(row()).not.toMatch(/ · (?:0|$)/);
  view.unmount();
});

it.each(["succeeded", "failed", "cancelled"] as const)("hides empty output for %s, retains running output and truncation evidence", (status) => {
  let update!: (value: ActionRun) => void;
  const run = renderRun(coded("running"), "en-US");
  function Surface() { const [value, set] = useState(run); update = set; return <RunOutput run={value} defaultOpen />; }
  const view = renderWithApp(<Surface />);
  expect(document.querySelector('[data-slot="run-output"]') !== null).toBe(true);
  expect(view.container.textContent).toContain("Läuft – neue Zeilen");
  act(() => update({ ...run, status }));
  expect(document.querySelector('[data-slot="run-output"]') === null).toBe(true);
  act(() => update({ ...run, status, outputTruncated: true }));
  expect(document.querySelector('[data-slot="run-output"]') !== null).toBe(true);
  act(() => update({ ...run, status, output: [{ at, stream: "stdout", text: "example output" }] }));
  expect(document.querySelector('[data-slot="run-output"]') !== null).toBe(true);
  expect(view.container.textContent).toContain("example output");
});

it("shows a step-less terminal result in the centre without a zero-percent progress bar", async () => {
  const adapter = createDemoAdapter(), run = coded();
  adapter.listActions = async () => [run];
  adapter.subscribeActions = (listener) => { listener([run]); return () => {}; };
  adapter.observeActionDetail = vi.fn(() => () => {});
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  renderWithApp(<ActivityCenter />, adapter);
  await waitFor(() => expect(document.querySelector("#run-example-run") !== null).toBe(true));
  const row = document.querySelector("#run-example-run")!;
  expect(row.textContent).toContain("Erfolgreich");
  expect(row.querySelector('[role="progressbar"]') === null).toBe(true);
  expect(row.textContent).not.toMatch(/0\s*%/);
});
