// Source: docklet-next/issues/224
// Revision: 75e73d5c4ffcd78b47ebaa166579079c02f70c8f
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original project update conflict through HTTP mapping and the stack editor.

import { EditorView } from "@codemirror/view";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { DockletAdapter } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { createStackAuthoring } from "@/data/http/stack-authoring";
import { renderRoutes } from "@/test/render";
import StackPage from "../detail/StackPage";

const toastError = vi.hoisted(() => vi.fn());
vi.mock("sonner", async (load) => {
  const actual = await load<typeof import("sonner")>();
  return { ...actual, toast: Object.assign(actual.toast, { error: toastError }) };
});
afterEach(() => { simulationStore.reset(); localStorage.clear(); setAppLocale("de"); toastError.mockReset(); });

it.each(["en-US", "de"] as const)("explains the project volume clash and keeps the stack draft (%s)", async (locale) => {
  setAppLocale(locale);
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const base = createDemoAdapter();
  const getCapability: DockletAdapter["getCapability"] = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
  // project.workspaceError returns this volume.ExistsError problem for a rename clash.
  const fetch = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ status: 409, title: "Conflict", detail: "A volume with this name already exists", code: "volume.exists" }), { status: 409, headers: { "Content-Type": "application/problem+json" } }));
  const up = vi.fn(async () => "run");
  const authoring = createStackAuthoring({ baseUrl: "/api", client: { fetch, token: async () => "example" }, capability: getCapability, up, labels: async () => [], changed: vi.fn(), unauthorized: vi.fn() });
  const { router } = renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"], { ...base, kind: "http", getStack: async (id) => ({ ...await base.getStack(id), id: "atlas~media" }), getCapability, saveStackFiles: authoring.saveStackFiles });
  const editor = await screen.findByRole("textbox", { name: locale === "de" ? "Inhalt von compose.yaml" : "Content of compose.yaml" });
  const view = EditorView.findFromDOM(editor)!;
  const draft = `${view.state.doc.toString()}\n# retained volume namespace draft`;
  await act(async () => { view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: draft } }); });
  const save = screen.getByRole("button", { name: locale === "de" ? "Speichern" : "Save" });
  await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
  await userEvent.click(save);
  await waitFor(() => expect(toastError).toHaveBeenCalledOnce());
  expect(toastError).toHaveBeenCalledWith(locale === "de" ? "Ein Volume mit diesem Namen existiert bereits auf diesem Host." : "A volume with this name already exists on this host.", expect.anything());
  expect(view.state.doc.toString()).toBe(draft);
  await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
  expect(router.state.location.pathname).toBe("/stacks/media");
  expect(fetch).toHaveBeenCalledOnce();
  expect(fetch.mock.calls[0]![0]).toBe("/api/environments/atlas/projects/media");
  expect(fetch.mock.calls[0]![1]?.method).toBe("PUT");
  expect(JSON.parse(String(fetch.mock.calls[0]![1]?.body)).composeContent).toBe(draft);
  expect(up).not.toHaveBeenCalled();
});
