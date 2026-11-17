// Source: docklet-next/issues/509
// Revision: fb1306767563a1e0d7bd028aaef362cca38166b6
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original bar presentation proofs for host card history.

import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";

import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const history = (id: string) => document.querySelector<HTMLElement>(`[data-host-card='${id}'] [data-slot="sparkline"]`);

it("draws the host CPU history as bars in the host colour, muted when offline", async () => {
  renderWithApp(<DashboardPage />);
  const online = HOSTS.find((h) => h.status === "online")!;
  const offline = HOSTS.find((h) => h.status === "offline")!;
  const chart = await waitFor(() => history(online.id) ?? Promise.reject(new Error("history missing")));
  const bars = [...chart.querySelectorAll("rect")];
  expect(bars.length > 1).toBe(true);
  expect(chart.querySelector("path") === null).toBe(true);
  expect(chart.getAttribute("role")).toBe("img");
  expect(chart.getAttribute("aria-label")).toBe(`CPU ${online.name}`);
  expect(bars.every((bar) => bar.style.fill === online.color)).toBe(true);
  const muted = history(offline.id);
  expect(muted !== null).toBe(true);
  expect([...muted!.querySelectorAll("rect")].every((bar) => bar.style.fill === "var(--muted-foreground)")).toBe(true);
});
