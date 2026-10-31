// Source: docklet-next/issues/84
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original authenticated v3 onboarding view tests.

import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "@/api/generated/schema";
import { setAppLocale } from "@/app/i18n/locale-store";
import { mapHost } from "@/data/http/hosts";
import { renderWithApp } from "@/test/render";

import { ConnectStep } from "./ConnectSteps";
import { ConnectionStep } from "./ConnectionStep";
import { freshDraft, planFromDraft } from "./onboarding-draft";

const draft = freshDraft({ projectDir: "/srv/projects", installDir: "/srv/projects/agent", connection: "edge", puid: "", pgid: "", detectSocketGroup: true }, []);
const report: components["schemas"]["VersionInfo"] = {
  agentContractVersion: 3, minimumAgentContractVersion: 3, agentContractFingerprint: "example", toolsImageOverride: false,
  currentVersion: "0.1.0", displayVersion: "0.1.0", goVersion: "go1.27.2", nodeVersion: "24",
  revision: "example", shortRevision: "example", isSemverVersion: true, updateAvailable: false,
  onboardingChecks: { projectDirectory: "/srv/projects", ownerUid: 123, ownerGid: 456, writable: false, socketGroup: 998 },
};
const row: components["schemas"]["EnvironmentEnvironment"] = {
  id: "example", name: "Example", color: null, apiUrl: "edge://example", enabled: true, isEdge: true, status: "online",
  agentReport: { reportedAt: "2026-10-17T12:00:00Z", version: report },
};

afterEach(() => setAppLocale("de"));
describe.each(["de", "en-US"] as const)("onboarding agent checks (%s)", (locale) => {
  it("renders authenticated v3 owner, effective write access and socket group", () => {
    setAppLocale(locale);
    renderWithApp(<ConnectStep draft={draft} host={mapHost(row)} hubUrl="http://hub.example.com" />);
    expect(screen.getByText(/123:456.*123:456.*nicht schreibbar|123:456.*123:456.*not writable/) !== null).toBe(true);
    expect(screen.getByText(/GID 998/) !== null).toBe(true);
    expect(screen.queryByText(/PUID\/PGID des Agents|Agent PUID\/PGID/) === null).toBe(true);
  });

  it("keeps absent checks unknown and describes health-run recovery after an upgrade", () => {
    setAppLocale(locale);
    const host = mapHost({ ...row, status: "update-required", agentReport: { ...row.agentReport!, version: { ...report, onboardingChecks: undefined } } });
    renderWithApp(<ConnectStep draft={draft} host={host} hubUrl="http://hub.example.com" />);
    expect(screen.queryByText(/GID 998/) === null).toBe(true);
    expect(screen.getByText(/nächste Health-Lauf.*2 Minuten|next health run.*2 minutes/) !== null).toBe(true);
    expect(screen.queryByText(/Example ist verbunden|Example is connected/) === null).toBe(true);
  });

  it("explains a rejected token without completing onboarding", () => {
    setAppLocale(locale);
    const host = mapHost({ ...row, status: "token-rejected", agentReport: undefined });
    renderWithApp(<ConnectStep draft={draft} host={host} hubUrl="http://hub.example.com" />);
    expect(screen.getByText(/Installiere das neu ausgestellte Token|Install the newly issued token/) !== null).toBe(true);
    expect(screen.queryByText(/Example ist verbunden|Example is connected/) === null).toBe(true);
  });

  it("warns about HTTP and proxy sign-in without offering mTLS", () => {
    setAppLocale(locale);
    const view = renderWithApp(<ConnectionStep draft={draft} errors={{}} showErrors={false} locked={false} hubUrl="http://hub.example.com" />);
    expect(screen.getByText(/HTTP.*vertrauenswürdiges Netz|HTTP.*trusted network/) !== null).toBe(true);
    expect(screen.getByText(/\/api\/environments\/pair.*\/api\/tunnel\//) !== null).toBe(true);
    expect(view.container.textContent?.toLowerCase().includes("mtls")).toBe(false);

  });
});

it("keeps manual socket group zero in the installation plan", () => {
  expect(planFromDraft({ ...draft, detectSocket: false, socketGid: "0" }, "https://hub.example.com", "UTC").socketGid).toBe(0);
});
