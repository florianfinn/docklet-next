// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original stack authoring workspace tests for issues #138 and #139.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StackCreatedError, type DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

const toastError = vi.hoisted(() => vi.fn());
vi.mock("sonner", async (load) => {
  const actual = await load<typeof import("sonner")>();
  return { ...actual, toast: Object.assign(actual.toast, { error: toastError }) };
});

beforeEach(() => { simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { simulationStore.reset(); localStorage.clear(); toastError.mockReset(); });

const routes = [{ path: "/stacks/new", element: <StackPage /> }, { path: "/stacks/:stackId", element: <StackPage /> }];
const asHttp = (overrides: Partial<DockletAdapter>): DockletAdapter => ({ ...createDemoAdapter(), kind: "http", ...overrides });

describe("stack authoring in production mode", () => {
  it("offers docker run conversion but no one-time import", async () => {
    renderRoutes(routes, ["/stacks/new"], asHttp({}));
    expect(await screen.findByRole("button", { name: /docker run/ }) !== null).toBe(true);
    expect(screen.queryByRole("button", { name: /GitHub \/ Docker Hub/ }) === null).toBe(true);
  });

  it("opens the created stack when its first deploy was not admitted", async () => {
    const deployStack = vi.fn(async () => { throw new StackCreatedError("atlas~p1", new AdapterOutcomeError({ kind: "conflict", code: "run.busy", activeRunId: "r1" }), "refused"); });
    const { router } = renderRoutes(routes, ["/stacks/new"], asHttp({ deployStack }));
    await userEvent.click(await screen.findByRole("button", { name: "Deploy" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/stacks/atlas~p1"));
    expect(toastError).toHaveBeenCalledWith(expect.stringContaining("Stack angelegt, aber der Deploy wurde nicht gestartet"));
  });

  it("keeps the new draft when the create outcome is unknown", async () => {
    const deployStack = vi.fn(async () => { throw new AdapterOutcomeError({ kind: "error", code: "stack.create-unknown" }); });
    const { router } = renderRoutes(routes, ["/stacks/new"], asHttp({ deployStack }));
    await userEvent.click(await screen.findByRole("button", { name: "Deploy" }));
    await waitFor(() => expect(toastError).toHaveBeenCalledWith(expect.stringContaining("Der Hub hat die Änderung nicht bestätigt")));
    expect(router.state.location.pathname).toBe("/stacks/new");
  });

  it("never claims a not-started deploy when the first admission is unknown", async () => {
    const deployStack = vi.fn(async () => { throw new StackCreatedError("atlas~p1", new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }), "unknown"); });
    const { router } = renderRoutes(routes, ["/stacks/new"], asHttp({ deployStack }));
    await userEvent.click(await screen.findByRole("button", { name: "Deploy" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/stacks/atlas~p1"));
    expect(toastError).toHaveBeenCalledWith(expect.stringContaining("Der Hub hat nicht bestätigt, ob der Deploy gestartet wurde"));
    expect(toastError.mock.calls.some(([m]) => String(m).includes("nicht gestartet"))).toBe(false);
  });

  it("disables deploy with a reason when the host descriptor refuses it", async () => {
    const getCapability = vi.fn(async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: operationId !== "deployStack", disabledReasonCode: "capability.unavailable" }));
    renderRoutes(routes, ["/stacks/new"], asHttp({ getCapability }));
    const deploy = await screen.findByRole("button", { name: "Deploy" });
    await waitFor(() => expect((deploy as HTMLButtonElement).disabled).toBe(true));
    expect(deploy.getAttribute("title") !== null).toBe(true);
    expect(getCapability.mock.calls.some(([op, ref]) => op === "deployStack" && ref?.kind === "host")).toBe(true);
  });

  it("blocks Save of changed labels when label writes are refused", async () => {
    const getCapability = vi.fn(async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: operationId !== "setStackLabels", disabledReasonCode: "capability.unavailable" }));
    renderRoutes(routes, ["/stacks/media?tab=editor"], asHttp({ getCapability }));
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" })).getByRole("radio", { name: "Easy" }));
    await userEvent.click(within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getByRole("button", { name: /Stack/ }));
    await userEvent.type(await screen.findByPlaceholderText("z. B. produktiv"), "NeuesLabel");
    await userEvent.click(screen.getByRole("button", { name: "Label hinzufügen" }));
    const save = screen.getByRole("button", { name: "Speichern" });
    await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(true));
    expect(save.getAttribute("title") !== null).toBe(true);
  });
});
