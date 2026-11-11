// Source: docklet-next/issues/398
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that an unavailable host's repository is not read.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import type { BackupTarget, BackupTargetInput } from "@/domain/settings";
import { BackupTargetsSection } from "../sections/BackupTargetsSection";
import type { Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { RepositoryCard } from "./RepositoryCard";

it("R2-i an offline host's repository card reads nothing and shows the host unavailable; online reads once", async () => {
  const adapter = createDemoAdapter();
  const [first] = await adapter.listHosts();
  const host: Host = { ...first!, status: "offline" };
  const read = vi.spyOn(adapter, "getBackupRepository");
  let setHost!: (next: Host) => void;
  function Probe() {
    const [current, set] = useState(host);
    setHost = set;
    return <RepositoryCard host={current} recovery={() => {}} />;
  }
  renderWithApp(<Probe />, adapter);
  expect((await screen.findByRole("status")).textContent).toMatch(/unavailable|nicht verfügbar/i);
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(read).not.toHaveBeenCalled();
  act(() => setHost({ ...host, status: "online" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
});

it("R1B-7 R1B-9 a warm card of a host going offline shows unavailable above its retained details, a target save reads nothing, recovery reads once", async () => {
  const adapter = createDemoAdapter();
  const [first] = await adapter.listHosts();
  const host: Host = { ...first!, status: "online" };
  const read = vi.spyOn(adapter, "getBackupRepository");
  let setHost!: (next: Host) => void;
  let client!: QueryClient;
  function Probe() {
    const [current, set] = useState(host);
    setHost = set;
    client = useQueryClient();
    return <RepositoryCard host={current} recovery={() => {}} />;
  }
  renderWithApp(<Probe />, adapter);
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.queryByText(/·/)).not.toBeNull());
  act(() => setHost({ ...host, status: "offline" }));
  expect((await screen.findByRole("status", {}, { timeout: 2_000 })).textContent).toMatch(/unavailable|nicht verfügbar/i);
  // The target dialog's save invalidates the repository read; an unavailable host is not read.
  await act(async () => { await client.invalidateQueries({ queryKey: ["backup-repository"] }); });
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(read).toHaveBeenCalledTimes(1);
  act(() => setHost({ ...host, status: "online" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
});

afterEach(() => simulationStore.reset());

it("R2-i a target saved while a host is offline shows its delivery pending and reads no repository; online reads once", async () => {
  simulationStore.set((state) => ({ ...state, latency: "instant", live: false }));
  const hosts = [{ ...HOSTS[0]!, id: "0", name: "hub", status: "online" }, { ...HOSTS[0]!, id: "first", name: "first", status: "offline" }] as Host[];
  const targets: BackupTarget[] = [];
  const demo = createDemoAdapter();
  const create = vi.fn(async (input: BackupTargetInput) => {
    // The hub stores the target; delivery to the offline host stays pending.
    const target = { id: "t-nas", ...input, hasSecret: false, createdAt: "2026-11-11T00:00:00Z", configVersion: 1, checks: [], delivery: [{ hostId: "0", state: "current" }, { hostId: "first", state: "pending" }] } as BackupTarget;
    targets.push(target);
    return target;
  });
  const adapter = {
    ...demo,
    listHosts: async () => hosts.map((host) => ({ ...host })),
    listBackupTargets: async () => targets.map((target) => ({ ...target })),
    createBackupTarget: create,
    getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }),
  };
  const read = vi.spyOn(adapter, "getBackupRepository");
  let client!: QueryClient;
  function Probe() { client = useQueryClient(); return null; }
  renderWithApp(<><Probe /><BackupTargetsSection /></>, adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Ziel hinzufügen" }));
  const field = (id: string) => document.getElementById(id) as HTMLInputElement;
  await userEvent.type(field("tg-name"), "Archive");
  await userEvent.type(field("tg-server"), "nas.example.com");
  await userEvent.type(field("tg-share"), "backups");
  await userEvent.type(field("tg-path"), "docklet");
  await userEvent.click(screen.getByRole("button", { name: "Übernehmen" }));
  await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
  expect(await screen.findByText(/first: Zustellung der aktuellen Einstellungen ausstehend/, {}, { timeout: 2_000 })).toBeTruthy();
  expect(read.mock.calls.some(([hostId]) => hostId === "first")).toBe(false);
  hosts[1] = { ...hosts[1]!, status: "online" };
  await act(async () => { await client.invalidateQueries({ queryKey: ["hosts"] }); });
  await waitFor(() => expect(read.mock.calls.filter(([hostId]) => hostId === "first")).toHaveLength(1));
});
