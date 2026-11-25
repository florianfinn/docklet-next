// Source: docklet-next/issues/624
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M41 coverage: version hints follow the reported agent version and a settled self-update.

import { act, cleanup, render, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { MemoryRouter } from "react-router";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { catalogs } from "@/app/i18n/catalog";
import type { SelfUpdateCheck } from "@/domain/self-update";
import type { SystemHints } from "@/domain/stack-updates";
import { selfUpdateQueryKey } from "@/shared/components/updates/use-self-update-check";
import { HostUpdateSettings } from "./LiveUpdatesSection";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const hints = (version: string): SystemHints => ({ hubVersion: "1.7.0", images: [
  { kind: "agent", reference: `ghcr.io/example/agent:${version}`, currentVersion: version, state: version === "1.7.0" ? "current" : "available", requiresHubUpdate: false, manualOnly: true },
] });
const check = (phase?: "running" | "settled"): SelfUpdateCheck => ({ version: "1.5.0", eligibility: { state: "current", manual: [] }, ...(phase ? { operation: { opId: "op", runId: "run", phase, targetVersion: "1.7.0" } } : {}) } as SelfUpdateCheck);

function setup(getSystemImageHints: DockletAdapter["getSystemImageHints"]) {
  const adapter: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", getCapability: vi.fn(async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true })),
    getSelfUpdate: vi.fn(async () => check()), getSystemImageHints, listHostRestorePoints: vi.fn(async () => []) };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = (host: { identity: string; agentVersion: string }) => <QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter><HostUpdateSettings hostId="example-host" name="Example host" host={host} /></MemoryRouter></IntlProvider></AdapterContext.Provider></QueryClientProvider>;
  return { client, view };
}

it("reads the hints again when the host reports the updated agent version", async () => {
  let version = "1.5.0";
  const read = vi.fn(async () => hints(version));
  const { view } = setup(read);
  const { rerender } = render(view({ identity: "registration-1", agentVersion: "1.5.0" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  version = "1.7.0";
  rerender(view({ identity: "registration-1", agentVersion: "1.7.0" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
});

it("reads the hints once more when an observed self-update settles at the same version", async () => {
  const read = vi.fn(async () => hints("1.5.0"));
  const { client, view } = setup(read);
  const host = { identity: "registration-1", agentVersion: "1.5.0" };
  render(view(host));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  const key = selfUpdateQueryKey("example-host", host);
  act(() => { client.setQueryData(key, check("running")); });
  await act(async () => { await Promise.resolve(); });
  expect(read).toHaveBeenCalledTimes(1);
  act(() => { client.setQueryData(key, check("settled")); });
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  // A later identical answer is no new settlement.
  act(() => { client.setQueryData(key, check("settled")); });
  await act(async () => { await Promise.resolve(); });
  expect(read).toHaveBeenCalledTimes(2);
});
