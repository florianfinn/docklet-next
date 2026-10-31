// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W3 intent, preserved fields, receipt navigation and W4 Compose gestures.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { ActionDetailState, DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import { renderRoutes, renderWithApp } from "@/test/render";
import { RecreateDialog } from "./RecreateDialog";
import { recreateDraft, recreateEdit, recreateIntent } from "./recreate-edit";
import { useContainerMenu } from "./container-menu";
import { ComposeContainerHint } from "./ComposeContainerHint";

const c = { ...STANDALONE.find((item) => item.origin === "standalone")!, id: "0~old", hostId: "0" };
const config = { id: "old", name: "app", image: "example/app:1", running: true, environment: ["EXAMPLE=value"], hostConfig: { restartPolicy: { name: "always" }, portBindings: { "80/tcp": [{ hostIp: "192.0.2.10", hostPort: "8080" }] } } };

it("W3 retries an unchanged edit with the same key and creates a new key for a changed edit", async () => {
  const inner = createDemoAdapter();
  const recreate = vi.fn().mockRejectedValue(new AdapterOutcomeError({ kind: "error", code: "container.recreate-recovered", status: 409 }));
  const adapter: DockletAdapter = { ...inner, getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }), getContainerEditConfig: async () => config, recreateContainer: recreate };
  renderWithApp(<RecreateDialog container={c} open onOpenChange={() => {}} />, adapter);
  const image = await screen.findByRole("textbox", { name: "Image" });
  await userEvent.clear(image); await userEvent.type(image, "example/app:2");
  expect(screen.getByText(/Die Neuerstellung ersetzt die Docker-ID/) !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Neu erstellen" }));
  await waitFor(() => expect(recreate).toHaveBeenCalledTimes(1));
  await waitFor(() => expect((screen.getByRole("button", { name: "Neu erstellen" }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(screen.getByRole("button", { name: "Neu erstellen" }));
  await waitFor(() => expect(recreate).toHaveBeenCalledTimes(2));
  expect(recreate.mock.calls[0]![2]).toBe(recreate.mock.calls[1]![2]);
  expect(recreate.mock.calls[0]![1]).toEqual({ image: "example/app:2" });
  await userEvent.type(image, "1");
  await userEvent.click(screen.getByRole("button", { name: "Neu erstellen" }));
  await waitFor(() => expect(recreate).toHaveBeenCalledTimes(3));
  expect(recreate.mock.calls[2]![2]).not.toBe(recreate.mock.calls[1]![2]);
});

it("W3 navigates using a retained replacement ID, including after a detail read failure", async () => {
  let receive: ((state: ActionDetailState) => void) | undefined;
  const adapter: DockletAdapter = { ...createDemoAdapter(), getCapability: async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }), getContainerEditConfig: async () => config, recreateContainer: async () => "run",
    observeActionDetail: (_host, _run, listener) => { receive = listener; return () => { receive = undefined; }; },
  };
  const { router } = renderRoutes([{ path: "/containers/:containerId", element: <RecreateDialog container={c} open onOpenChange={() => {}} /> }], ["/containers/old"], adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Neu erstellen" }));
  await waitFor(() => expect(typeof receive).toBe("function"));
  act(() => receive?.({ status: "error", error: new Error("read unavailable") }));
  expect(screen.getByText(/Der Lauf konnte nicht aktualisiert/) !== null).toBe(true);
  act(() => receive?.({ status: "ready", run: { id: "run", kind: "recreate", title: { code: "runMessages.value", params: { value: "Recreate" } }, target: { code: "runMessages.value", params: { value: "app" } }, hostId: "0", containerId: "0~old", services: [], status: "succeeded", steps: [], progress: 100, startedAt: "2026-10-31T00:00:00Z", log: [], output: [], recreateResult: { oldId: "old", newId: "replacement", oldName: "app", newName: "renamed" } } }));
  await waitFor(() => expect(router.state.location.pathname).not.toBe("/containers/old"));
  expect(router.state.location.pathname).toBe("/containers/0~replacement");
});

it("W3 only changes edited configuration sections and owns a key per submitted edit", () => {
  const initial = recreateDraft(config);
  expect(recreateEdit(initial, initial)).toEqual({ image: "example/app:1" });
  expect(recreateEdit(initial, { ...initial, name: "renamed", environment: "EXAMPLE=changed" })).toEqual({ image: "example/app:1", name: "renamed", environment: ["EXAMPLE=changed"] });
  const next = recreateIntent(vi.fn().mockReturnValueOnce("first").mockReturnValueOnce("second"));
  expect(next({ image: "example/app:1" })).toBe("first");
  expect(next({ image: "example/app:1" })).toBe("first");
  expect(next({ image: "example/app:2" })).toBe("second");
});

it("W4 offers stack redeploy and service restart instead of Compose-child recreate", async () => {
  const inner = createDemoAdapter();
  const stack = (await inner.listStacks())[0]!;
  const run = vi.spyOn(inner, "runStackCommand").mockResolvedValue("run");
  renderWithApp(<ComposeContainerHint c={{ ...c, id: "child", origin: "compose", stackId: stack.id, stackName: stack.name, serviceName: stack.services[0]!.name }} />, inner);
  expect(screen.queryByRole("button", { name: "Neu erstellen" }) === null).toBe(true);
  await userEvent.click(await screen.findByRole("button", { name: "Stack neu bereitstellen" }));
  await waitFor(() => expect(run).toHaveBeenCalledExactlyOnceWith(stack.id, "redeploy"));
  expect((screen.getByRole("button", { name: "Service neu starten" }) as HTMLButtonElement).disabled).toBe(false);
});

function ComposeMenu() {
  const groups = useContainerMenu({ ...c, origin: "compose" }, { onOpen: () => {}, onEdit: () => {} }, false);
  return <>{groups.flat().map((action) => <button key={action.id}>{action.label}</button>)}</>;
}
it("W4 omits the container edit/recreate entry for Compose children", () => {
  renderWithApp(<ComposeMenu />);
  expect(screen.queryByRole("button", { name: "Bearbeiten & neu erstellen" }) === null).toBe(true);
});
