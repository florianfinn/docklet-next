// Source: docklet-next/docs/design/stack-authoring.md
// Revision: a8a1c0ce786f0ce57e5d27b8a4a510b639a60277
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original regressions for M7 review finding PR151-R3-01, after the reviewer's recovery probe.

import { EditorView } from "@codemirror/view";
import { useQueryClient } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DockletAdapter } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { queryKeys } from "@/data/query-keys";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

beforeEach(() => { simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { simulationStore.reset(); localStorage.clear(); });

async function setup() {
  const base = createDemoAdapter();
  const stored = { ...(await base.getStack("media")), env: "EXAMPLE=stored\n" };
  let available = true;
  const saveStackFiles = vi.fn(async (_id: string, _compose: string, _env: string) => {});
  const setStackLabels = vi.fn(async (_id: string, _labels: string[]) => {});
  const getStack: DockletAdapter["getStack"] = async () => ({ ...stored, compose: available ? stored.compose : "", env: available ? stored.env : "", ...(available ? {} : { filesUnavailable: true }) });
  let refetch = async () => {};
  function Page() {
    const client = useQueryClient();
    refetch = async () => { await client.refetchQueries({ queryKey: queryKeys.stack("media") }); };
    return <StackPage />;
  }
  const getCapability: DockletAdapter["getCapability"] = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
  const { router } = renderRoutes([{ path: "/stacks/:stackId", element: <Page /> }], ["/stacks/media?tab=editor"], { ...base, kind: "http", getStack, saveStackFiles, setStackLabels, getCapability });
  return {
    stored, saveStackFiles, setStackLabels,
    setAvailable: (value: boolean) => { available = value; },
    refetch: () => act(async () => { await refetch(); }),
    tab: (tab: string) => act(async () => { await router.navigate(`/stacks/media?tab=${tab}`); }),
  };
}
const editor = (name = "compose.yaml") => screen.getByRole("textbox", { name: `Inhalt von ${name}` });
const save = () => screen.getByRole("button", { name: /^Speichern$/ }) as HTMLButtonElement;

describe("a loaded draft across a later read failure", () => {
  it("keeps edits, blocks writes while unreadable and saves the edited files after recovery", async () => {
    const f = await setup();
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const view = EditorView.findFromDOM(editor())!;
    await act(async () => { view.dispatch({ changes: { from: view.state.doc.length, insert: "\n# kept edit\n" } }); });
    const edited = view.state.doc.toString();
    await f.tab("overview");
    f.setAvailable(false);
    await f.refetch();
    await f.tab("editor");
    expect(EditorView.findFromDOM(editor())!.state.doc.toString()).toBe(edited);
    expect((await screen.findByRole("alert")).textContent).toContain("Dein Entwurf bleibt erhalten");
    await waitFor(() => expect(save().disabled).toBe(true));
    await act(async () => { fireEvent.keyDown(editor(), { key: "s", code: "KeyS", ctrlKey: true }); });
    expect(f.saveStackFiles).toHaveBeenCalledTimes(0);
    f.setAvailable(true);
    await f.refetch();
    await waitFor(() => expect(save().disabled).toBe(false));
    await userEvent.click(save());
    await waitFor(() => expect(f.saveStackFiles).toHaveBeenCalledWith("media", edited, f.stored.env));
  });

  it("keeps an added label across the failure", async () => {
    const f = await setup();
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" })).getByRole("radio", { name: "Easy" }));
    await userEvent.click(within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getByRole("button", { name: /Stack/ }));
    await userEvent.type(screen.getByPlaceholderText("z. B. produktiv"), "example-edited");
    await userEvent.click(screen.getByRole("button", { name: "Label hinzufügen" }));
    f.setAvailable(false);
    await f.refetch();
    f.setAvailable(true);
    await f.refetch();
    await waitFor(() => expect(save().disabled).toBe(false));
    await userEvent.click(save());
    await waitFor(() => expect(f.setStackLabels).toHaveBeenCalledWith("media", expect.arrayContaining(["example-edited"])));
    expect(f.saveStackFiles).toHaveBeenCalledWith("media", f.stored.compose, f.stored.env);
  });

  it("bases edits made while unreadable on the last loaded files", async () => {
    const f = await setup();
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    f.setAvailable(false);
    await f.refetch();
    await screen.findByRole("alert");
    const view = EditorView.findFromDOM(editor())!;
    await act(async () => { view.dispatch({ changes: { from: view.state.doc.length, insert: "# during failure\n" } }); });
    f.setAvailable(true);
    await f.refetch();
    await waitFor(() => expect(save().disabled).toBe(false));
    await userEvent.click(save());
    await waitFor(() => expect(f.saveStackFiles).toHaveBeenCalledTimes(1));
    expect(f.saveStackFiles.mock.calls[0]![1]).toBe(`${f.stored.compose}# during failure\n`);
  });
});
