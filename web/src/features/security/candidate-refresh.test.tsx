// Source: docklet-next/issues/438
// Revision: 057901783492684c0a9e9073e8d1f71fb2032f87
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original candidate refresh regressions (M21 R1-S12W B1, B2), from the reviewer's probes.
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { advisoryKey, type AdvisoryRead } from "@/data/vulnerability-advisory";
import { decodeAdvisory } from "@/data/http/vulnerability-advisory";
import { decodeCandidates } from "@/data/http/vulnerability-candidates";
import fixture from "@/data/http/testdata/vulnerability-candidates.json";
import advisoryFixture from "@/data/http/testdata/vulnerability-advisory.json";
import type { CandidateResults } from "@/data/vulnerability-candidates";
import type { Topic } from "@/data/adapter";
import { renderWithApp } from "@/test/render";
import { AdvisoryContent } from "./AdvisoryContent";

afterEach(() => { setAppLocale("de"); localStorage.clear(); });
function setup() {
  setAppLocale("en-US");
  const adapter = createDemoAdapter(), data = decodeCandidates(fixture), result = data.results[0]!;
  result.proof!.completedAt = new Date().toISOString();
  const detail = decodeAdvisory(advisoryFixture, "CVE-2099-0001"), o = detail.occurrences[1]!;
  Object.assign(o, { vulnerabilityId: "CVE-2026-1234", imageId: result.consumer.imageId, pkgName: "example", installedVersion: "1.0", class: "os-pkgs", type: "alpine", containers: [{ id: "consumer", name: "Example consumer", state: "running" }], updates: [{ containerId: "consumer", state: "available", version: "2.0" }] });
  detail.occurrences = [o]; detail.advisory!.vulnerabilityId = o.vulnerabilityId;
  const read: AdvisoryRead = { key: advisoryKey(o.vulnerabilityId), hosts: [{ hostId: "example", detail }] };
  adapter.getVulnerabilityCandidates = vi.fn(async () => structuredClone(data));
  adapter.installContainerUpdate = vi.fn(async () => "update-run");
  adapter.getCapability = vi.fn(async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }));
  adapter.scanVulnerabilityCandidates = vi.fn(async () => "candidate-run");
  adapter.observeActionDetail = vi.fn(() => () => {});
  const listeners = new Set<(topics: Topic[]) => void>();
  adapter.subscribeChanges = (listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
  const show = () => renderWithApp(<AdvisoryContent data={read} hosts={new Map()} onRetry={() => {}} />, adapter);
  return { adapter, data, show, listeners };
}

// Stream topics arrive at least every 30 s; candidate reads call the registry.
it("never re-reads candidates on hosts or security stream topics", async () => {
  const f = setup(); f.show();
  await screen.findByRole("button", { name: "Update to verified image" });
  const before = vi.mocked(f.adapter.getVulnerabilityCandidates).mock.calls.length;
  for (let i = 0; i < 10; i++) await act(async () => { f.listeners.forEach((l) => l(["hosts"])); f.listeners.forEach((l) => l(["security"])); await Promise.resolve(); });
  expect(vi.mocked(f.adapter.getVulnerabilityCandidates).mock.calls.length - before).toBe(0);
});

async function heldRefresh(f: ReturnType<typeof setup>) {
  let release!: (data: CandidateResults) => void;
  f.adapter.getVulnerabilityCandidates = vi.fn(() => new Promise<CandidateResults>((resolve) => { release = resolve; }));
  await userEvent.click(screen.getByRole("button", { name: "Check candidates" }));
  await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalled());
  return () => act(async () => { release(f.data); });
}

it("keeps the verified claim and offer while a refresh is in flight", async () => {
  const f = setup(); f.show();
  await screen.findByRole("button", { name: "Update to verified image" });
  const release = await heldRefresh(f);
  const state = { claim: screen.queryByText(/closes this vulnerability/) !== null, offer: screen.queryByRole("button", { name: "Update to verified image" }) !== null, notChecked: screen.queryByText("Not checked yet") !== null };
  await release();
  expect(state).toEqual({ claim: true, offer: true, notChecked: false });
});

it("keeps an open confirmation dialog across a refresh when a run finishes", async () => {
  const f = setup();
  const run: CandidateResults["runs"][number] = { id: "auto", activityId: "auto-run", trigger: "automatic", status: "running", startedAt: new Date().toISOString(), targets: 1, scanned: 0, cached: 0, failed: 0, deferred: 0, intelligence: "available" };
  f.data.runs = [run];
  const observers = new Map<string, (detail: unknown) => void>();
  f.adapter.observeActionDetail = vi.fn((_host: string, id: string, listener: (detail: never) => void) => { observers.set(id, listener as (detail: unknown) => void); return () => {}; }) as never;
  f.show();
  await userEvent.click(await screen.findByRole("button", { name: "Update to verified image" }));
  await screen.findByRole("alertdialog");
  let release!: (data: CandidateResults) => void;
  f.adapter.getVulnerabilityCandidates = vi.fn(() => new Promise<CandidateResults>((resolve) => { release = resolve; }));
  await waitFor(() => expect(observers.has("auto-run")).toBe(true));
  await act(async () => { observers.get("auto-run")!({ status: "ready", run: { ...run, status: "success" } }); });
  await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalled());
  const during = screen.queryByRole("alertdialog") !== null;
  await act(async () => { release({ ...f.data, runs: [{ ...run, status: "completed" }] }); });
  const after = screen.queryByRole("alertdialog");
  expect([during, after !== null, after ? !within(after).getByRole("button", { name: "Confirm update" }).hasAttribute("disabled") : false]).toEqual([true, true, true]);
});
