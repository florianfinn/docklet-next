// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original page-level recreate ownership across the replacement's identity change.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { ActionDetailState, DockletAdapter } from "@/data/adapter";
import type { Topic } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import type { ContainerInfo } from "@/domain/model";
import { renderRoutes, renderWithApp } from "@/test/render";
import { useCapability } from "@/data/use-capability";
import ContainerPage from "./ContainerPage";
import { RecreateRun } from "./RecreateRun";

const old: ContainerInfo = { ...STANDALONE.find((item) => item.origin === "standalone")!, id: "0~old", hostId: "0", name: "app" };
const config = { id: "old", name: "app", image: "example/app:1", running: true, environment: [], hostConfig: { restartPolicy: { name: "always" }, portBindings: {} } };

// The old ID leaves the inventory before the run settles; the page still follows
// the run, refreshes the inventory and opens the replacement with the form closed.
it("W3 navigates to the replacement after the old container left the list", async () => {
  let rows: ContainerInfo[] = [old];
  let receive: ((state: ActionDetailState) => void) | undefined;
  const changes = new Set<(topics: Topic[]) => void>();
  const inner = createDemoAdapter();
  const adapter: DockletAdapter = {
    ...inner,
    listContainers: async () => rows,
    subscribeChanges: (listener) => { changes.add(listener); return () => { changes.delete(listener); }; },
    getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }),
    getContainerEditConfig: async () => config,
    recreateContainer: async () => "run",
    observeActionDetail: (_host, _run, listener) => { receive = listener; return () => { receive = undefined; }; },
  };
  const { router } = renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/0~old"], adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Bearbeiten & neu erstellen" }));
  await userEvent.click(await screen.findByRole("button", { name: "Neu erstellen" }));
  await waitFor(() => expect(typeof receive).toBe("function"));
  rows = [];
  act(() => changes.forEach((listener) => listener(["containers"])));
  await screen.findByText("Container nicht gefunden");
  rows = [{ ...old, id: "0~replacement", name: "renamed" }];
  act(() => receive?.({ status: "ready", run: { id: "run", kind: "recreate", title: { code: "runMessages.value", params: { value: "Recreate" } }, target: { code: "runMessages.value", params: { value: "app" } }, hostId: "0", containerId: "0~old", services: [], status: "succeeded", steps: [], progress: 100, startedAt: "2026-10-31T00:00:00Z", log: [], output: [], recreateResult: { oldId: "old", newId: "replacement", oldName: "app", newName: "renamed" } } }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/containers/0~replacement"));
  expect(await screen.findByRole("heading", { name: "renamed" }) !== null).toBe(true);
  expect(screen.queryByRole("dialog") === null).toBe(true);
});

it("W3 shows recreate run steps from the catalogue in German", async () => {
  let receive: ((state: ActionDetailState) => void) | undefined;
  const adapter: DockletAdapter = { ...createDemoAdapter(), observeActionDetail: (_host, _run, listener) => { receive = listener; return () => { receive = undefined; }; } };
  renderRoutes([{ path: "*", element: <RecreateRun hostId="0" runId="run" /> }], ["/containers/0~old"], adapter);
  await waitFor(() => expect(typeof receive).toBe("function"));
  act(() => receive?.({ status: "ready", run: { id: "run", kind: "recreate", title: { code: "runMessages.value", params: { value: "Recreate" } }, target: { code: "runMessages.value", params: { value: "app" } }, hostId: "0", containerId: "0~old", services: [], status: "running", steps: [{ key: "other", label: { code: "runMessages.value", params: { value: "Renaming container" } }, state: "done" }, { key: "other", label: { code: "runMessages.value", params: { value: "Creating replacement" } }, state: "running" }], progress: 50, startedAt: "2026-10-31T00:00:00Z", log: [], output: [] } }));
  expect(screen.queryByText(/Container wird umbenannt/) !== null).toBe(true);
  expect(screen.queryByText(/Ersatz wird erstellt/) !== null).toBe(true);
  expect(screen.queryByText(/Renaming container/) === null).toBe(true);
});

function Reasons() {
  const assign = useCapability("setContainerOrigin", { kind: "container", id: "0~old" });
  const edit = useCapability("recreateContainer", { kind: "container", id: "0~old" });
  return <><p data-testid="assign">{assign.reason}</p><p data-testid="edit">{edit.reason}</p></>;
}

it("W2 explains a locked origin assignment with its own text", async () => {
  const adapter: DockletAdapter = { ...createDemoAdapter(), kind: "http", getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: false, disabledReasonCode: "container.origin-locked" }) };
  renderWithApp(<Reasons />, adapter);
  await waitFor(() => expect(screen.getByTestId("assign").textContent?.startsWith("Die Erkennung bestimmt")).toBe(true));
  expect(screen.getByTestId("edit").textContent?.startsWith("Bearbeiten ist durch Herkunft")).toBe(true);
});
