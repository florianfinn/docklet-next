// Source: docklet-next/issues/601
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original effective-agent permission matrix and tailnet address rendering proofs.

import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { components } from "@/api/generated/schema";
import { mapHost } from "@/data/http/hosts";
import { AgentReportSection } from "@/features/hosts/AgentReportSection";
import { HostCard } from "@/features/hosts/HostCard";
import { renderWithApp } from "@/test/render";
import { peerHost, at } from "./tailnet.test-support";

const version: components["schemas"]["VersionInfo"] = { agentContractVersion: 23, agentContractFingerprint: "example", minimumAgentContractVersion: 3, toolsImageOverride: false, currentVersion: "1.5.0", displayVersion: "1.5.0", goVersion: "go1.27.4", nodeVersion: "24", revision: "example", shortRevision: "example", isSemverVersion: true, updateAvailable: false };
afterEach(() => setAppLocale("de"));

it.each(["en-US", "de"] as const)("uses the effective agent identity and write result in %s", (locale) => {
  setAppLocale(locale);
  for (const agentUid of [0, undefined, 1000]) for (const writable of [true, false]) {
    const host = mapHost({ ...peerHost, agentReport: { reportedAt: at, version: { ...version, onboardingChecks: { projectDirectory: "/srv/projects", ownerUid: 0, ownerGid: 100, socketGroup: null, agentUid, agentGid: agentUid, writable } } } });
    const warn = !writable && agentUid !== 0;
    expect(host.agentReport?.note).toBe(warn ? "root-owned" : "ok");
    const view = renderWithApp(<AgentReportSection host={host} />);
    expect(view.container.textContent?.includes("PUID/PGID")).toBe(warn);
    if (warn) {
      expect(view.container.textContent).toContain("0:100");
      expect(view.container.textContent).toContain(locale === "de" ? "ändere den Eigentümer" : "change the owner");
    }
    view.unmount();
  }
});

it.each(["en-US", "de"] as const)("shows the tailnet DNS name and IPv4 with a secondary route hint in %s", (locale) => {
  setAppLocale(locale);
  const host = mapHost({ ...peerHost, tailnet: { ...peerHost.tailnet!, dnsName: "agent.example.com", addresses: ["192.0.2.42", "2001:db8::42"] } });
  expect(host.tailnet?.addresses).toEqual(["192.0.2.42", "2001:db8::42"]);
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={0} containers={0} allIds={[host.id]} />);
  expect(view.container.textContent).toContain("agent.example.com · 192.0.2.42 · Tailscale");
  expect(view.container.textContent).toContain(locale === "de" ? "Keine öffentliche Hub-Adresse" : "No public hub address");
});

it("keeps older host peers without address fields valid", () => {
  const host = mapHost(peerHost);
  expect(host.tailnet?.dnsName).toBeUndefined();
  expect(host.tailnet?.addresses).toBeUndefined();
});

it("rejects malformed peer fields and effective agent IDs", () => {
  for (const tailnet of [{ dnsName: 42 }, { dnsName: "bad name" }, { addresses: ["not-an-IP"] }, { addresses: Array(5).fill("192.0.2.1") }]) {
    expect(() => mapHost({ ...peerHost, tailnet: { ...peerHost.tailnet!, ...tailnet } } as Parameters<typeof mapHost>[0])).toThrow("hosts.invalid-peer");
  }
  for (const agentUid of [-1, "root"]) {
    const row = { ...peerHost, agentReport: { reportedAt: at, version: { ...version, onboardingChecks: { projectDirectory: "/srv/projects", ownerUid: 0, ownerGid: 0, writable: true, agentUid } } } };
    expect(() => mapHost(row as Parameters<typeof mapHost>[0])).toThrow("hosts.invalid-checks");
  }
});
