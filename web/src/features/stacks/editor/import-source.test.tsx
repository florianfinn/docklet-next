// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original UI lifecycle and role boundary proofs. M19 web consumer integration.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import type { ImportResult } from "@/data/adapter";
import { renderWithApp } from "@/test/render";
import { ImportSourceDialog } from "./ImportSourceDialog";
const proposal: ImportResult = { title: "late-proposal", composeContent: "services: {}", envNames: [], found: ["import.found.compose"], missing: ["import.missing.variables"] };
beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());
function deferred() { let resolve!: (value: ImportResult) => void; const promise = new Promise<ImportResult>((done) => { resolve = done; }); return { promise, resolve }; }
it.each(["ref", "tab", "close", "draft", "identity"])("late import is fenced after %s change", async (change) => {
  const adapter = createDemoAdapter(), held = deferred(), apply = vi.fn();
  adapter.importSource = vi.fn(() => held.promise);
  const listeners = new Set<() => void>(); let version = 0;
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => version === 0, listen: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; } });
  function Harness() { const [open, setOpen] = useState(true), [draftKey, setKey] = useState("draft-a"); return <><button onClick={() => setOpen(true)}>reopen</button><button onClick={() => setKey("draft-b")}>change draft</button><ImportSourceDialog open={open} draftKey={draftKey} onOpenChange={setOpen} onApply={apply} /></>; }
  renderWithApp(<Harness />, adapter);
  await userEvent.type(await screen.findByLabelText("Quelle"), "example/app");
  await userEvent.click(screen.getByRole("button", { name: "Quelle auswerten" }));
  await waitFor(() => expect(adapter.importSource).toHaveBeenCalledTimes(1));
  expect((screen.getByRole("button", { name: "Übernehmen" }) as HTMLButtonElement).disabled).toBe(true);
  if (change === "ref") await userEvent.type(screen.getByLabelText("Quelle"), "-new");
  if (change === "tab") await userEvent.click(screen.getByRole("tab", { name: "Docker Hub" }));
  if (change === "close") { await userEvent.keyboard("{Escape}"); await userEvent.click(screen.getByText("reopen")); }
  if (change === "draft") await act(() => { screen.getByText("change draft").click(); });
  if (change === "identity") await act(() => { version++; listeners.forEach((fn) => fn()); });
  await act(async () => { held.resolve(proposal); await held.promise; });
  expect(screen.queryByText("late-proposal") === null).toBe(true);
  expect(apply).toHaveBeenCalledTimes(0);
  if (change !== "identity") expect((screen.getByRole("button", { name: "Übernehmen" }) as HTMLButtonElement).disabled).toBe(true);
});
it("a later request owns Apply and renders contract found and missing codes in German", async () => {
  const adapter = createDemoAdapter(), first = deferred(), second = deferred(), apply = vi.fn(); let calls = 0;
  adapter.importSource = () => ++calls === 1 ? first.promise : second.promise;
  renderWithApp(<ImportSourceDialog open onOpenChange={() => {}} onApply={apply} />, adapter);
  await userEvent.type(screen.getByLabelText("Quelle"), "example/first"); await userEvent.click(screen.getByRole("button", { name: "Quelle auswerten" }));
  await userEvent.type(screen.getByLabelText("Quelle"), "-second"); await userEvent.click(screen.getByRole("button", { name: "Quelle auswerten" }));
  await act(async () => { second.resolve({ ...proposal, title: "owned-proposal" }); await second.promise; });
  await act(async () => { first.resolve(proposal); await first.promise; });
  expect(screen.queryByText("late-proposal") === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Übernehmen" })); expect(apply.mock.calls[0]?.[0].title).toBe("owned-proposal");
});
