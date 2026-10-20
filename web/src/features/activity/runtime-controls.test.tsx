// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterContext } from "@/data/use-adapter";
import { useActionRuns } from "@/data/live";
import { renderRun } from "@/data/message-code";
import { withRunGate } from "@/data/run-gate";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { envelope, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "@/data/http/runtime-commands.test-support";
import { StackHeader } from "@/features/stacks/detail/StackHeader";
import { useContainerMenu } from "@/features/containers/container-menu";
import type { ContainerInfo } from "@/domain/model";
import { ActionsMenu } from "@/shared/components/action-menu";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { RuntimeControls } from "./RuntimeControls";
import { RunStatusLine } from "./RunFocus";
import { ActionToastView } from "./ActionToastView";
import { toast } from "sonner";
import { actionDialogStore } from "./action-dialog-store";
import { presentationStore } from "./run-presentation";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { deferred } from "@/data/http/activity-reader.test-support";
import { ActionToasts } from "./ActionToasts";

const clients: QueryClient[] = [];
afterEach(() => { clients.splice(0).forEach((client) => client.clear()); act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); }); vi.restoreAllMocks(); });
function mount(ui: ReactNode, adapter: DockletAdapter, locale: "en-US" | "de" = "en-US") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } }); clients.push(client);
  return render(<AdapterContext.Provider value={withRunGate(adapter)}><QueryClientProvider client={client}><IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><MemoryRouter><TooltipProvider>{ui}</TooltipProvider></MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
}
function VisibleRun() { const run = useActionRuns()[0]; return run ? <RunStatusLine run={run} /> : null; }
function ContainerMenu({ container }: { container: ContainerInfo }) { return <ActionsMenu groups={useContainerMenu(container, { onOpen() {}, onEdit() {} }, false)} label="Example commands" />; }

it.each(["start", "stop", "restart", "redeploy"] as const)("existing stack %s button is keyboard-operable and accepted admission is never success", async (command) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { type: command === "redeploy" ? "project_redeploy" : "project_deploy", resourceType: "project", resourceId: "example-project" });
  const demo = createDemoAdapter(); const base = await demo.getStack("media"); const stack = { ...base, id: "edge~example-project", hostId: "edge", services: base.services.map((service) => ({ ...service, state: command === "start" ? "stopped" as const : "running" as const })) };
  const view = mount(<><StackHeader stack={stack as never} host={undefined} /><VisibleRun /></>, f.adapter);
  try {
    const names = { start: "Start", stop: "Stop", restart: "Restart", redeploy: "Pull & redeploy" };
    const button = screen.getByRole("button", { name: names[command] });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false)); button.focus(); await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(f.commands()).toHaveLength(1));
    await waitFor(() => expect(screen.queryByText("Execution unconfirmed") !== null).toBe(true));
    expect(screen.queryByText("Completed successfully")).toBeNull();
    const path = command === "start" ? "up" : command === "stop" ? "down" : command;
    expect(f.commands()[0]!.url.pathname).toBe(`/api/environments/edge/projects/example-project/${path}`);
  } finally { view.unmount(); }
});
it.each(["start", "stop", "restart"] as const)("existing container menu %s uses the single-container command route", async (command) => {
  const f = runtimeFixture(); f.state.run = snapshot("0");
  const base = (await createDemoAdapter().listContainers())[0]!;
  const container: ContainerInfo = { ...base, id: "0~example-container", hostId: "0", origin: "standalone", stackId: undefined, state: command === "start" ? "stopped" : "running" };
  const adapter = { ...f.adapter, listStacks: async () => [] };
  const view = mount(<ContainerMenu container={container} />, adapter);
  try {
    await userEvent.click(screen.getByRole("button", { name: "Example commands" }));
    const label = { start: "Start", stop: "Stop", restart: "Restart" }[command];
    const item = await screen.findByRole("menuitem", { name: label });
    await waitFor(() => expect(item.getAttribute("data-disabled") === null).toBe(true));
    item.focus(); await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(f.commands()).toHaveLength(1)); expect(f.commands()[0]!.url.pathname).toBe(`/api/environments/0/containers/example-container/${command}`);
  } finally { view.unmount(); }
});
it.each(["en-US", "de"] as const)("%s manual recovery requires reason plus explicit acknowledgment and never labels released unknown success", async (locale) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "executor-continuity";
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), locale);
  f.state.read = (_url, init) => init.method === "POST" ? envelope({ ...f.state.run, status: "failed" }) : envelope(runtimeDetail(f.state.run, { recovery: f.state.recovery }));
  const view = mount(<RuntimeControls run={run} />, f.adapter, locale);
  try {
    const button = screen.getByRole("button", { name: locale === "de" ? "Gesperrtes Ziel freigeben" : "Release blocked target" });
    expect(button.hasAttribute("disabled")).toBe(true);
    await userEvent.type(screen.getByRole("textbox"), "Executor stopped and state inspected"); expect(button.hasAttribute("disabled")).toBe(true);
    const ack = screen.getByRole("checkbox"); ack.focus(); await userEvent.keyboard(" ");
    expect(button.hasAttribute("disabled")).toBe(false); await userEvent.click(button);
    await waitFor(() => expect(f.calls.filter(({ url }) => url.pathname.endsWith("/release"))).toHaveLength(1));
    expect(f.commands()).toHaveLength(0);
  } finally { view.unmount(); }
});
it("ordinary user has no administrator recovery control; pending unknown has no loss alert in every toast design", async () => {
  const f = runtimeFixture(); f.state.admin = false; f.state.run = snapshot("edge");
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), "en-US");
  const view = mount(<><RuntimeControls run={run} />{(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={run} design={design} detail="full" now={0} major={false} onClose={() => {}} onOpen={() => {}} />)}</>, f.adapter);
  try {
    expect(screen.queryByRole("button", { name: "Release blocked target" })).toBeNull();
    expect(screen.getAllByText("Execution unconfirmed").length).toBeGreaterThanOrEqual(3);
    expect(view.container.textContent).not.toContain("Authoritative executor evidence is unavailable");
    expect(view.container.textContent).not.toContain("Succeeded");
  } finally { view.unmount(); }
});
it("a current admission on the first observation offers its major dialog, then reconnect does not replay its failure", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { type: "project_redeploy", resourceType: "project", resourceId: "example" });
  const adapter = { ...f.adapter, getConnectedSettings: async () => ({ activity: { actionPresentation: "dialog" } }) as never };
  const shows = vi.spyOn(toast, "custom");
  const view = mount(<ActionToasts />, adapter);
  try {
    await act(async () => { await f.adapter.runStackCommand("edge~example", "redeploy"); });
    expect(actionDialogStore.get().runId).toBe(RUN_ID);
    f.state.run.status = "failed"; f.state.run.outcomeState = "known";
    await act(async () => { await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    const count = shows.mock.calls.length;
    await act(async () => { await f.adapter.listActions(); await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    expect(shows.mock.calls.length).toBe(count); expect(f.commands()).toHaveLength(1);
  } finally { view.unmount(); }
});

it.each(["stack", "container"] as const)("late %s command callback cannot toast or publish into a replacement session", async (kind) => {
  const f = runtimeFixture(); const errors = vi.spyOn(toast, "error"); const reached = deferred<void>(), release = deferred<void>();
  let settled: Promise<unknown> | undefined;
  f.state.command = async () => { reached.resolve(); await release.promise; return envelope({ code: "run.started", activityId: RUN_ID, activity: f.state.run }, 202); };
  function Command() {
    const stack = useStackCommands(), container = useContainerCommands();
    return <button onClick={() => { settled = (kind === "stack" ? stack : container).run("0~example", "start"); }}>Start example</button>;
  }
  const view = mount(<Command />, f.adapter);
  try {
    await userEvent.click(screen.getByRole("button", { name: "Start example" })); await reached.promise;
    await act(async () => { await f.adapter.logout(); release.resolve(); await settled; });
    expect(errors).not.toHaveBeenCalled(); expect(f.commands()).toHaveLength(1);
    expect(f.commands()[0]!.init.signal?.aborted).toBe(true);
  } finally { release.resolve(); view.unmount(); }
});

it("remote unconfirmed cancellation shows intent separately without claiming completion or outcome loss", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { cancelRequested: true });
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), "en-US");
  const view = mount(<RuntimeControls run={run} />, f.adapter);
  try {
    expect(view.container.textContent).toContain("Executor confirmation is pending");
    expect(view.container.textContent).toContain("Cancellation was requested");
    expect(view.container.textContent).not.toContain("The workflow has stopped");
    expect(view.container.textContent).not.toContain("Authoritative executor evidence is unavailable");
    expect(screen.getByRole("button", { name: "Request cancellation" }).hasAttribute("disabled")).toBe(true);
    expect(f.commands()).toHaveLength(0);
  } finally { view.unmount(); }
});
