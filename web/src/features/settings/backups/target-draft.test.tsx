// Source: docklet-next/docs/design/backup-completion.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regression for the draft target test in the target dialog (BC17, #329).

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import type { Host } from "@/domain/model";
import type { BackupTarget } from "@/domain/settings";
import { renderWithApp } from "@/test/render";
import { TargetDialog } from "./TargetDialog";

afterEach(() => simulationStore.reset());
function setup() {
  simulationStore.set((state) => ({ ...state, latency: "instant", live: false }));
  const probe = vi.fn(async () => ({ ok: true }));
  const hosts = ["0", "first"].map((id) => ({ ...HOSTS[0]!, id, name: id, status: "online" }) as Host);
  const adapter = { ...createDemoAdapter(), listHosts: async () => hosts, testBackupTargetDraft: probe, getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }) };
  return { adapter, probe };
}
const field = (id: string) => document.getElementById(id) as HTMLInputElement;
// The probe host comes from the host list, which the demo adapter serves asynchronously.
async function test() {
  const button = screen.getByRole("button", { name: "Verbindung testen" }) as HTMLButtonElement;
  await waitFor(() => expect(button.disabled).toBe(false));
  await userEvent.click(button);
}

it("tests an unsaved definition and hides the result after any change", async () => {
  const { adapter, probe } = setup();
  renderWithApp(<TargetDialog onClose={() => {}} saved={() => {}} />, adapter);
  await userEvent.type(field("tg-name"), "Archive");
  await userEvent.type(field("tg-server"), "nas.example.com");
  await userEvent.type(field("tg-share"), "backups");
  await userEvent.type(field("tg-path"), "docklet");
  await test();
  await waitFor(() => expect(screen.queryByRole("status")?.textContent).toBe("Test erfolgreich"));
  expect(probe.mock.calls[0]).toEqual([{ target: expect.objectContaining({ kind: "smb", server: "nas.example.com", share: "backups", path: "docklet" }), probeHostId: "0" }]);
  await userEvent.type(field("tg-path"), "/old");
  expect(screen.queryByRole("status") === null).toBe(true);
});

it("tests an edited target with its stored secret and shows a typed failure", async () => {
  const { adapter, probe } = setup();
  probe.mockResolvedValueOnce({ ok: false, code: "backup.target.permission-denied" } as never);
  const target = { id: "t-nas", name: "NAS", kind: "smb", server: "nas.example.com", share: "backups", path: "docklet", username: "backup", options: "vers=3.1.1", hasSecret: true, createdAt: "2026-11-01T00:00:00Z", configVersion: 1, checks: [], delivery: [] } as BackupTarget;
  renderWithApp(<TargetDialog target={target} onClose={() => {}} saved={() => {}} />, adapter);
  await test();
  await waitFor(() => expect(screen.queryByRole("status")?.textContent).toContain("Test fehlgeschlagen"));
  const [input] = probe.mock.calls[0] as unknown as [{ target: Record<string, unknown>; targetId?: string }];
  expect([input.targetId, "secret" in input.target]).toEqual(["t-nas", false]);
});
