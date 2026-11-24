// Source: docklet-next/issues/616
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original host-identity ownership tests for the shared self-update check (M40 break-map row 3).

import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { createAppQueryClient } from "@/data/app-query-client";
import type { SelfUpdateCheck } from "@/domain/self-update";
import { useSelfUpdateCheck } from "./use-self-update-check";

afterEach(cleanup);
const answer = (version: string, candidate?: string): SelfUpdateCheck => ({ version, eligibility: candidate ? { state: "available", candidate: { version: candidate }, manual: [] } : { state: "current", manual: [] } });
function deferred<T>() { let resolve!: (v: T) => void; const promise = new Promise<T>((r) => { resolve = r; }); return { promise, resolve }; }
function Probe({ identity, version }: { identity: string; version: string }) {
  const { query } = useSelfUpdateCheck("7", { host: { identity, agentVersion: version }, enabled: true });
  return <p data-testid="probe">{query.data ? `${query.data.version}:${query.data.eligibility.candidate?.version ?? "none"}` : "pending"}</p>;
}
function mount(getSelfUpdate: DockletAdapter["getSelfUpdate"], client: QueryClient = createAppQueryClient()) {
  const adapter: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", getSelfUpdate };
  const view = (identity: string, version: string) => <QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><Probe identity={identity} version={version} /></AdapterContext.Provider></QueryClientProvider>;
  return { view, client };
}
// Identity strings follow the host mapper: registrationRevision|edgeSessionId|edgeAgentInstance|connectedAt.
const first = "3|session-a|instance-a|2026-11-25T10:00:00Z", replaced = "4|session-b|instance-b|2026-11-25T10:05:00Z";

it("row 3: a replacement registration reads afresh and never shows the superseded answer", async () => {
  const old = deferred<SelfUpdateCheck>(), fresh = deferred<SelfUpdateCheck>();
  const get = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
  const { view } = mount(get);
  const r = render(view(first, "1.5.0"));
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1));
  r.rerender(view(replaced, "1.5.0"));
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  await act(async () => { old.resolve(answer("1.5.0", "1.6.0")); });
  expect(screen.getByTestId("probe").textContent).toBe("pending");
  await act(async () => { fresh.resolve(answer("1.6.0")); });
  await waitFor(() => expect(screen.getByTestId("probe").textContent).toBe("1.6.0:none"));
});

it("row 3: a reconnect or a new reported version reads afresh within the freshness window", async () => {
  const get = vi.fn().mockResolvedValueOnce(answer("1.5.0", "1.6.0")).mockResolvedValueOnce(answer("1.6.0")).mockResolvedValueOnce(answer("1.6.0"));
  const { view } = mount(get);
  const r = render(view(first, "1.5.0"));
  await waitFor(() => expect(screen.getByTestId("probe").textContent).toBe("1.5.0:1.6.0"));
  r.rerender(view("3|session-a|instance-a|2026-11-25T10:09:00Z", "1.5.0"));
  await waitFor(() => expect(screen.getByTestId("probe").textContent).toBe("1.6.0:none"));
  r.rerender(view("3|session-a|instance-a|2026-11-25T10:09:00Z", "1.6.0"));
  await waitFor(() => expect(get).toHaveBeenCalledTimes(3));
});
