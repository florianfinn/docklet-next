// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original local host card proofs for M37 #579. M40 #616: the version moved into the header badge.

import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { mapHost } from "@/data/http/hosts";
import type { SystemComponent } from "@/domain/system";
import { renderWithApp } from "@/test/render";

import { HostCard } from "./HostCard";

const local = mapHost({ id: "0", name: "Local Docker", color: null, registrationRevision: 1, apiUrl: "https://hub.example.com", enabled: true, status: "online", isEdge: false });
const remote = mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
const component = (hostId: string): SystemComponent => ({
  hostId, role: "agent", metadataOnly: true, agentVersion: "v1.4.0", contractVersion: 21, toolsImage: "ghcr.io/example/tools:1.0.0", updateRequired: false,
  containerName: "", composeProject: "", image: { repository: "", tag: "", version: "", published: "" }, state: "unknown", installDir: "", insideProjectDir: false,
  labels: {}, ports: [], mounts: [], networks: [], restartPolicy: "", memLimitMb: 0, recentLog: [],
});

afterEach(() => { setAppLocale("de"); });

const labels = (container: HTMLElement) => [...container.querySelectorAll("dt")].map((dt) => dt.textContent ?? "");

it.each(["en-US", "de"] as const)("hides agent-only rows on the hub's own Docker and shows each version once as a badge (%s)", (locale) => {
  setAppLocale(locale);
  const en = locale === "en-US";
  const view = renderWithApp(<HostCard host={local} system={component("0")} stacks={1} containers={2} allIds={["0", "atlas"]} />);
  const shown = labels(view.container);
  for (const hidden of en ? ["Agent installation", "File IDs (PUID/PGID)", "Agent", "Contract version", "Agent version", "Address", "Hub version"] : ["Agent-Installation", "Datei-IDs (PUID/PGID)", "Agent", "Vertragsversion", "Agent-Version", "Adresse", "Hub-Version"])
    expect(shown.includes(hidden)).toBe(false);
  for (const visible of en ? ["Hub address", "Docker", "Project directory"] : ["Hub-Adresse", "Docker", "Projektverzeichnis"])
    expect(shown.includes(visible)).toBe(true);
  expect(view.container.querySelector("[data-slot=installed-version]")?.textContent).toBe("1.4.0");
  expect((view.container.textContent ?? "").split("1.4.0").length - 1).toBe(1);
  view.unmount();

  const other = renderWithApp(<HostCard host={remote} system={component("atlas")} stacks={1} containers={2} allIds={["0", "atlas"]} />);
  const remoteShown = labels(other.container);
  for (const kept of en ? ["Agent installation", "Address", "Contract version", "Docker"] : ["Agent-Installation", "Adresse", "Vertragsversion", "Docker"])
    expect(remoteShown.includes(kept)).toBe(true);
  expect(remoteShown.includes(en ? "Agent version" : "Agent-Version")).toBe(false);
  expect(other.container.querySelector("[data-slot=installed-version]")?.textContent).toBe("1.4.0");
});
