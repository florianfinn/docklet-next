// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4037e18b9c926793e2888952b4ec9c0805ecc62e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original workspace and capability status regressions for #183.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { CapabilityDescriptor } from "@/data/capability";
import { WorkspaceError } from "@/data/http/workspace";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { renderWithApp } from "@/test/render";
import { FolderView } from "./FolderView";
import { FileWorkspace } from "./FileWorkspace";
import type { Volume } from "@/domain/volumes";

afterEach(() => { localStorage.clear(); });

it("renders one workspace listing error with a retry action", async () => {
  const refetch = vi.fn();
  renderWithApp(<FolderView listing={{ isPending: false, isError: true, isPlaceholderData: false, error: new WorkspaceError("helper-incompatible"), refetch }} singleFile={false} onOpen={() => {}} onDownload={() => {}} onRename={() => {}} onDelete={() => {}} />);
  expect(screen.getAllByRole("alert").length).toBe(1);
  expect(screen.getByRole("alert").textContent).toContain("Hilfsimage ist inkompatibel");
  await userEvent.click(screen.getByRole("button", { name: "Erneut versuchen" }));
  expect(refetch).toHaveBeenCalledOnce();
});

it("presents bind mounts neutrally without trying workspace reads", async () => {
  const adapter = createDemoAdapter();
  adapter.listFiles = vi.fn(async () => { throw new WorkspaceError("forbidden"); });
  const volume = { id: "example~bind:/data", kind: "bind" } as Volume;
  renderWithApp(<FileWorkspace volume={volume} />, adapter);
  expect(screen.getByRole("status").textContent).toBe("Dateizugriff ist für Bind-Mounts nicht verfügbar.");
  expect(screen.queryByRole("alert") === null).toBe(true);
  expect(adapter.listFiles).not.toHaveBeenCalled();
});

for (const available of [false, true]) {
  it(`shows neutral loading while a capability descriptor is pending (${available})`, async () => {
    let resolve!: (descriptor: CapabilityDescriptor) => void;
    const pending = new Promise<CapabilityDescriptor>((done) => { resolve = done; });
    const adapter = { ...createDemoAdapter(), kind: "http" as const, getCapability: vi.fn(() => pending) };
    renderWithApp(<CapabilityGate operation="completeSetup"><p>Ready flow</p></CapabilityGate>, adapter);
    expect(screen.getByRole("status").textContent).toBe("Lädt");
    expect(screen.getByRole("status").getAttribute("aria-busy")).toBe("true");
    expect(screen.queryByText("Diese Funktion ist nicht verfügbar") === null).toBe(true);
    await waitFor(() => expect(adapter.getCapability).toHaveBeenCalled());
    await act(async () => { resolve({ operationId: "completeSetup", resourceRef: { kind: "hub", id: "0" }, available }); await pending; });
    if (available) expect(await screen.findByText("Ready flow") !== null).toBe(true);
    else expect(await screen.findByText("Diese Funktion ist nicht verfügbar") !== null).toBe(true);
  });
}
