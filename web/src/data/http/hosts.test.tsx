// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host HTTP mapping and report tests.

import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { components } from "@/api/generated/schema";
import { HostCard } from "@/features/hosts/HostCard";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp } from "@/test/render";

import { createHttpAdapter } from "./http-adapter";
import { mapHost } from "./hosts";

type Environment = components["schemas"]["EnvironmentEnvironment"];
const at = "2026-10-17T12:00:00Z";
const row: Environment = {
  id: "example-host", name: "Example host", color: null, apiUrl: "http://agent.example.com:3552", enabled: true, status: "online", isEdge: false, lastSeen: at,
};
const info: components["schemas"]["VersionInfo"] = {
  agentContractVersion: 3, agentContractFingerprint: "example", minimumAgentContractVersion: 3, toolsImageOverride: false,
  currentVersion: "v0.1.0", displayVersion: "0.1.0", goVersion: "go1.27.2", nodeVersion: "24", revision: "example", shortRevision: "example", isSemverVersion: true, updateAvailable: false,
  onboardingChecks: { projectDirectory: "/srv/projects", ownerUid: 1000, ownerGid: 1000, writable: false, socketGroup: 998 },
};

function fixture(rows: Environment[]) {
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const url = new URL(String(input), "https://hub.example.com");
    const data = url.pathname === "/api/capabilities" ? [{ operationId: "listHosts", available: true, resourceRef: { kind: "hub", id: "0" } }] : rows;
    return new Response(JSON.stringify({ success: true, data, ...(Array.isArray(data) && url.pathname !== "/api/capabilities" ? { pagination: { totalItems: data.length } } : {}) }), { headers: { "Content-Type": "application/json" } });
  });
  return { adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }), fetcher };
}

describe("host HTTP reads", () => {
  it.each(["online", "offline", "pending", "update-required", "token-rejected", "standby", "error"])("maps %s and last_seen without inventing a contact", async (status) => {
    const f = fixture([{ ...row, status }]);
    const [host] = await f.adapter.listHosts();
    expect(host?.status).toBe(["standby", "error"].includes(status) ? "offline" : status);
    expect(host?.lastSeen).toBe(at);
    expect(host?.connection).toBe("direct");
    expect(host?.demoAdded).toBeUndefined();
    expect(host?.detailsUnavailable).toBe(true);
  });

  it("maps Edge identity, missing contact and the agent's reported checks", async () => {
    const f = fixture([{ ...row, isEdge: true, status: "pending", lastSeen: undefined, agentReport: { reportedAt: at, version: info } }]);
    const [host] = await f.adapter.listHosts();
    expect(host?.connection).toBe("edge");
    expect(host?.address).toBe("");
    expect(host?.lastSeen).toBe("");
    expect(host?.agentVersion).toBe("v0.1.0");
    expect(host?.projectDir).toBe("/srv/projects");
    expect(host?.agentReport).toMatchObject({ reportedAt: at, projectDirOwner: { uid: 1000, gid: 1000 }, projectDirWritable: false, socketGid: 998, simulated: false });
    expect(host?.agentReport?.fileOwner).toBeUndefined();
    expect(host?.agentReport?.fileOwnerSource).toBeUndefined();
    expect(f.fetcher.mock.calls.every(([url]) => !String(url).includes("deployment"))).toBe(true);
  });

  it("does not turn malformed host data into an empty success", async () => {
    const f = fixture([{ ...row, status: "invented" }]);
    await expect(f.adapter.listHosts()).rejects.toMatchObject({ outcome: { kind: "error", code: "listHosts.mapping-error" } });
  });

  it.each(["de", "en-US"] as const)("describes connected update-required and pending reports accurately in %s", (locale) => {
    setAppLocale(locale);
    try {
      for (const status of ["update-required", "pending", "offline", "token-rejected"] as const) {
        const host = mapHost({ ...row, status, agentReport: { reportedAt: at, version: info } });
        const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={[host.id]} />);
        const report = locale === "de" ? /Bericht .*Agent-Update erforderlich/ : /Report .*agent update required/;
        const pending = locale === "de" ? /Bericht .*Kompatibilitätsprüfung ausstehend/ : /Report .*waiting for compatibility check/;
        const disconnected = /host currently disconnected|Host derzeit getrennt/;
        expect(view.container.textContent?.match(disconnected) !== null).toBe(status === "offline");
        if (status === "update-required") expect(view.container.textContent?.match(report) !== null).toBe(true);
        if (status === "token-rejected") expect(view.container.textContent?.match(/Bericht .*Agent-Token ungültig|Report .*agent token invalid/) !== null).toBe(true);
        if (status === "pending") expect(view.container.textContent?.match(pending) !== null).toBe(true);
        view.unmount();
      }
    } finally {
      setAppLocale("de");
    }
  });

  it.each(["de", "en-US"] as const)("shows rejected rotation credentials and a repair reason in %s", (locale) => {
    setAppLocale(locale);
    try {
      const host = mapHost({ ...row, status: "token-rejected", lastSeen: undefined });
      renderWithApp(<HostCard host={host} system={undefined} stacks={1} containers={3} allIds={[host.id]} />);
      expect(screen.queryByText(locale === "de" ? "Agent-Token ungültig" : "Agent token invalid") !== null).toBe(true);
      expect(screen.getByRole("status").textContent?.includes(locale === "de" ? "Installiere das neu ausgestellte Token" : "Install the newly issued token")).toBe(true);
      expect(screen.queryByText("Online") === null).toBe(true);
    } finally { setAppLocale("de"); }
  });

  it("renders a real report and update-required state with unknown fields absent", () => {
    const host = mapHost({ ...row, status: "update-required", lastSeen: undefined, agentReport: { reportedAt: at, version: info } });
    renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={[host.id]} />);
    expect(screen.queryByText("Update erforderlich") !== null).toBe(true);
    expect(screen.queryByText(/1000:1000.*nicht schreibbar/) !== null).toBe(true);
    expect(screen.queryByText(/GID 998/) !== null).toBe(true);
    expect(screen.queryByText("PUID/PGID des Agents") === null).toBe(true);
    expect(screen.queryByText(/Simulation/) === null).toBe(true);
  });
});

it("invalidates host reads on environment state and reconnect envelopes", () => {
  const source = {
    onopen: null as (() => void) | null,
    onmessage: null as ((event: MessageEvent) => void) | null,
    onerror: null as (() => void) | null,
    close: vi.fn(),
  };
  const adapter = createHttpAdapter({ eventSource: () => source });
  const changed = vi.fn();
  const stop = adapter.subscribeChanges(changed);
  source.onopen?.();
  expect(changed).toHaveBeenCalledWith(["events", "hosts", "stacks", "containers", "images", "volumes", "topology", "security"]);
  changed.mockClear();
  source.onmessage?.({ data: JSON.stringify({ channel: "environments", type: "snapshot", timestamp: at }), lastEventId: "1" } as MessageEvent);
  expect(changed).toHaveBeenCalledWith(["hosts", "stacks", "containers", "images", "volumes", "topology", "security"]);
  stop();
});
