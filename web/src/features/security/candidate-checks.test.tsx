// Source: docklet-next/issues/438
// Revision: cc93f43359147e8e128b2ac00bcd410c055eb423
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original candidate proof consumer and verified update integration. M26 #445: stack-only baseline capability read regression.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { catalogs } from "@/app/i18n/catalog";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { AdapterOutcomeError } from "@/data/capability";
import { advisoryKey, type AdvisoryRead } from "@/data/vulnerability-advisory";
import { decodeAdvisory } from "@/data/http/vulnerability-advisory";
import { decodeCandidates } from "@/data/http/vulnerability-candidates";
import fixture from "@/data/http/testdata/vulnerability-candidates.json";
import advisoryFixture from "@/data/http/testdata/vulnerability-advisory.json";
import type { CandidateResults, CandidateResult } from "@/data/vulnerability-candidates";
import type { ActionDetailState, Topic } from "@/data/adapter";
import { renderWithApp } from "@/test/render";
import { AdvisoryContent } from "./AdvisoryContent";
import { useCandidates } from "./use-candidates";
import { renderMessage } from "@/data/message-code";
import { mapActivityDetail } from "@/data/http/map-dto";

const manifest = fixture.results[0]!.identity.manifestDigest;
afterEach(() => { setAppLocale("de"); localStorage.clear(); });
function setup(stack = false, locale: "en-US" | "de" = "en-US") {
  setAppLocale(locale);
  const adapter = createDemoAdapter(), data = decodeCandidates(fixture), result = data.results[0]!;
  result.proof!.completedAt = new Date().toISOString();
  if (stack) { result.consumer.projectId = "project"; result.consumer.service = "app"; }
  const detail = decodeAdvisory(advisoryFixture, "CVE-2099-0001"), o = detail.occurrences[1]!;
  Object.assign(o, { vulnerabilityId: "CVE-2026-1234", imageId: result.consumer.imageId, pkgName: "example", installedVersion: "1.0", class: "os-pkgs", type: "alpine", containers: [{ id: "consumer", name: "Example consumer", state: "running", ...(stack ? { projectId: "project", project: "Example stack", service: "app" } : {}) }], updates: [{ containerId: "consumer", state: "available", version: "2.0" }] });
  detail.occurrences = [o]; detail.advisory!.vulnerabilityId = o.vulnerabilityId;
  const read: AdvisoryRead = { key: advisoryKey(o.vulnerabilityId), hosts: [{ hostId: "example", detail }] };
  adapter.getVulnerabilityCandidates = vi.fn(async () => structuredClone(data));
  adapter.scanVulnerabilityCandidates = vi.fn(async () => "candidate-run");
  adapter.installContainerUpdate = vi.fn(async () => "update-run");
  adapter.installStackUpdates = vi.fn(async () => "update-run");
  adapter.getUpdateBaseline = vi.fn(async () => ({ targetIdentity: "b".repeat(64), savedRevision: "a".repeat(64), observedRevision: "a".repeat(64), available: true, autoExcluded: false }));
  adapter.getCapability = vi.fn(async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }));
  const observers = new Map<string, (state: ActionDetailState) => void>();
  adapter.observeActionDetail = vi.fn((_host, id, listener) => { observers.set(id, listener); return () => { observers.delete(id); }; });
  const listeners = new Set<(topics: Topic[]) => void>();
  adapter.subscribeChanges = (listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
  const show = () => renderWithApp(<AdvisoryContent data={read} hosts={new Map()} onRetry={() => {}} />, adapter);
  return { adapter, data, result, read, o, show, observers, listeners };
}
it.each(["closes", "unknown", "does-not-close", "failed", "deferred", "unchecked", "expired", "missing-proof", "running", "cancelled", "interrupted"])("CW-a verdict truth for %s", async (state) => {
  const f = setup();
  if (["closes", "unknown", "does-not-close"].includes(state)) f.result.verdicts[0]!.verdict = state as "closes";
  else if (state === "expired") f.result.proof!.completedAt = new Date(Date.now() - 86400001).toISOString();
  else if (state === "missing-proof") delete f.result.proof;
  else f.result.state = state as CandidateResult["state"];
  f.show(); await screen.findByText("Introduces");
  if (state === "closes") { await screen.findByRole("button", { name: "Update to verified image" }); expect(screen.queryByText(/closes this vulnerability/) !== null).toBe(true); expect(screen.queryByText("Update available: 2.0") === null).toBe(true); }
  else { expect(screen.queryByText(state === "does-not-close" ? /does not close/ : state === "running" ? "Candidate check running" : state === "cancelled" ? "Candidate check cancelled" : state === "interrupted" ? "Candidate check interrupted" : state === "failed" ? /Candidate check unavailable/ : "Not checked yet") !== null).toBe(true); expect(screen.queryByRole("button", { name: "Update to verified image" }) === null).toBe(true); expect(screen.queryByText(/ closes this vulnerability/) === null).toBe(true); }
});
it.each([false, true])("CW-b/CW-d confirms consumer and manifest before the exact update (stack=%s)", async (stack) => {
  const f = setup(stack); f.show();
  await userEvent.click(await screen.findByRole("button", { name: "Update to verified image" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog.textContent).toContain(stack ? "Example stack / app" : "Example consumer"); expect(dialog.textContent).toContain(manifest);
  await userEvent.click(within(dialog).getByRole("button", { name: "Confirm update" }));
  if (stack) expect(f.adapter.installStackUpdates).toHaveBeenCalledWith("example~project", { service: "app", expectedRevision: "a".repeat(64), expectedManifest: manifest });
  else expect(f.adapter.installContainerUpdate).toHaveBeenCalledWith("example~consumer", { expectedManifest: manifest });
});
it.each(["en-US", "de"] as const)("CW-c shows candidate-changed and withdraws stale proof in %s", async (locale) => {
  const f = setup(false, locale);
  f.adapter.installContainerUpdate = vi.fn(async () => { throw new AdapterOutcomeError({ kind: "error", code: "update.candidate-changed", status: 409 }); });
  f.show(); const t = catalogs[locale].security.candidates;
  await userEvent.click(await screen.findByRole("button", { name: t.update }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: t.confirmAction }));
  await screen.findByText(catalogs[locale].images.errors["update_candidate-changed"]);
  await act(async () => { await Promise.resolve(); });
  expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("button", { name: t.update }) === null).toBe(true);
  expect(screen.queryByText(/ closes this vulnerability| schließt diese Schwachstelle \(verifiziert/) === null).toBe(true);
});
it("CW-f labels automatic runs and keeps deferred targets with a manual action", async () => {
  const f = setup(); f.result.state = "deferred"; delete f.result.proof;
  f.data.runs = [{ id: "automatic", activityId: "auto-run", trigger: "automatic", status: "partial", startedAt: new Date().toISOString(), targets: 15, scanned: 10, cached: 0, failed: 0, deferred: 5, intelligence: "available" }];
  f.show(); await screen.findByText(/Automatic candidate check/);
  expect(screen.queryByText("Not checked yet") !== null).toBe(true);
  expect(screen.queryByText(/10 scanned · 0 cached · 0 failed · 5 not checked yet/) !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Check candidates" }));
  await waitFor(() => expect(f.adapter.scanVulnerabilityCandidates).toHaveBeenCalledWith("example", { advisory: "CVE-2026-1234" }));
});
it("follows manual completion through the owned activity path and rereads results", async () => {
  const f = setup(); f.data.results = []; f.show();
  await userEvent.click(await screen.findByRole("button", { name: "Check candidates" }));
  await waitFor(() => expect(f.observers.has("candidate-run")).toBe(true));
  const before = vi.mocked(f.adapter.getVulnerabilityCandidates).mock.calls.length;
  await act(async () => { f.observers.get("candidate-run")!({ status: "ready", run: mapActivityDetail({ activity: { id: "candidate-run", environmentId: "example", type: "vulnerability.candidate-scan", status: "success", startedAt: new Date().toISOString(), createdAt: new Date().toISOString() }, messages: [], stepTrace: [], output: [], outputTruncated: false }) }); });
  await waitFor(() => expect(vi.mocked(f.adapter.getVulnerabilityCandidates).mock.calls.length).toBeGreaterThan(before));
});
it("renders introduced findings and neutral catalogue reasons in EN and DE", async () => {
  const f = setup(); f.result.state = "failed"; f.result.code = "candidate.unavailable"; f.result.reason = "registry.authentication-required"; delete f.result.proof;
  f.show(); await screen.findByText(catalogs["en-US"].images.errors["registry_authentication-required"]);
  expect(screen.queryByText("Introduces") !== null).toBe(true); expect(screen.queryByText(/CVE-2026-5678/) !== null).toBe(true);
});
it.each(["access.admin-required", "feature-disabled"])("CW-h hides check and update actions when %s", async (reason) => {
  const f = setup(); f.adapter.getCapability = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: false, disabledReasonCode: reason });
  f.show(); await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalled());
  await screen.findByText(reason === "feature-disabled" ? catalogs["en-US"].images.errors["feature-disabled"] : catalogs["en-US"].containers.operationReasons.accessAdminRequired);
  expect(screen.queryByRole("button", { name: "Check candidates" }) === null).toBe(true);
  expect(screen.queryByRole("button", { name: "Update to verified image" }) === null).toBe(true);
});
it("keeps ignore input and focus during a candidate result refresh", async () => {
  const f = setup();
  const run: CandidateResults["runs"][number] = { id: "auto", activityId: "auto-run", trigger: "automatic", status: "running", startedAt: new Date().toISOString(), targets: 1, scanned: 0, cached: 0, failed: 0, deferred: 0, intelligence: "available" };
  f.data.runs = [run];
  const observers = new Map<string, (detail: unknown) => void>();
  f.adapter.observeActionDetail = vi.fn((_host: string, id: string, listener: (detail: never) => void) => { observers.set(id, listener as (detail: unknown) => void); return () => {}; }) as never;
  f.show(); await screen.findByRole("button", { name: "Update to verified image" });
  const input = screen.getByLabelText("Reason (optional)"); await userEvent.type(input, "accepted risk");
  let release!: (data: CandidateResults) => void;
  f.adapter.getVulnerabilityCandidates = vi.fn(() => new Promise<CandidateResults>((resolve) => { release = resolve; }));
  // A finished run re-reads; stream topics do not.
  await waitFor(() => expect(observers.has("auto-run")).toBe(true));
  await act(async () => { observers.get("auto-run")!({ status: "ready", run: { ...run, status: "success" } }); });
  await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalled());
  expect([(input as HTMLInputElement).value, document.activeElement === input]).toEqual(["accepted risk", true]);
  await act(async () => { release(f.data); });
  expect([(input as HTMLInputElement).value, document.activeElement === input]).toEqual(["accepted risk", true]);
});
it.each(["advisory", "host", "logout", "close"])("CW-e rejects late candidate A after %s", async (change) => {
  const f = setup(); let release!: (data: CandidateResults) => void;
  const held = new Promise<CandidateResults>((resolve) => { release = resolve; });
  f.adapter.getVulnerabilityCandidates = vi.fn(async (host, selector) => host === "first" && selector.advisory === "A" ? held : { results: [], runs: [] });
  let version = 0, active = true; const listeners = new Set<() => void>();
  registerIdentityLifetime(f.adapter.subscribeChanges, { version: () => version, active: () => active, listen: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; } });
  function Reader({ host, advisory }: { host: string; advisory: string }) { const r = useCandidates(host, advisory); return <p data-testid="result">{r.data?.results[0]?.consumer.containerId ?? "empty"}</p>; }
  function Probe() { const [host, setHost] = useState("first"), [advisory, setAdvisory] = useState("A"), [open, setOpen] = useState(true); return <><button onClick={() => setHost("second")}>host</button><button onClick={() => setAdvisory("B")}>advisory</button><button onClick={() => setOpen(false)}>close</button>{open && <Reader host={host} advisory={advisory} />}</>; }
  renderWithApp(<Probe />, f.adapter);
  await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalledWith("first", { advisory: "A" }, expect.any(AbortSignal)));
  if (change === "logout") act(() => { version++; active = false; listeners.forEach((fn) => fn()); });
  else await userEvent.click(screen.getByRole("button", { name: change }));
  await act(async () => { release(f.data); await held; });
  expect(screen.queryByText("consumer") === null).toBe(true);
});

it("CW-e keeps a newer candidate refetch when an older read completes last", async () => {
  const f = setup(); let release!: (value: CandidateResults) => void;
  const old = structuredClone(f.data); old.results[0]!.consumer.containerId = "old-consumer";
  const held = new Promise<CandidateResults>((resolve) => { release = resolve; });
  f.adapter.getVulnerabilityCandidates = vi.fn().mockImplementationOnce(() => held).mockResolvedValue(f.data);
  function Probe() { const r = useCandidates("example", "CVE-2026-1234"); return <><button onClick={r.reload}>reload</button><p>{r.data?.results[0]?.consumer.containerId ?? "empty"}</p></>; }
  renderWithApp(<Probe />, f.adapter);
  await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalledTimes(1));
  await userEvent.click(screen.getByRole("button", { name: "reload" }));
  await screen.findByText("consumer");
  await act(async () => { release(old); await held; });
  expect(screen.queryByText("old-consumer") === null).toBe(true);
  expect(screen.queryByText("consumer") !== null).toBe(true);
});
it("withdraws the verified offer immediately on logout", async () => {
  const f = setup(); let version = 0, active = true; const listeners = new Set<() => void>();
  registerIdentityLifetime(f.adapter.subscribeChanges, { version: () => version, active: () => active, listen: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; } });
  f.show(); await screen.findByRole("button", { name: "Update to verified image" });
  act(() => { version++; active = false; listeners.forEach((fn) => fn()); });
  expect(screen.queryByRole("button", { name: "Update to verified image" }) === null).toBe(true);
  expect(screen.queryByRole("button", { name: "Check candidates" }) === null).toBe(true);
});
it("handles an asynchronous candidate-changed update outcome", async () => {
  const f = setup(); f.show();
  await userEvent.click(await screen.findByRole("button", { name: "Update to verified image" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Confirm update" }));
  await waitFor(() => expect(f.observers.has("update-run")).toBe(true));
  const run = mapActivityDetail({ activity: { id: "update-run", environmentId: "example", type: "image_patch", status: "failed", startedAt: new Date().toISOString(), createdAt: new Date().toISOString(), imageLifecycle: { runId: "update-run", code: "update.candidate-changed", verified: false, volumeDataIncluded: false } }, messages: [], stepTrace: [], output: [], outputTruncated: false });
  await act(async () => { f.observers.get("update-run")!({ status: "ready", run }); });
  await screen.findByText(catalogs["en-US"].images.errors["update_candidate-changed"]);
  await act(async () => { await Promise.resolve(); });
  expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("button", { name: "Update to verified image" }) === null).toBe(true);
});
it("shows and follows an automatic running candidate batch", async () => {
  const f = setup(); f.result.state = "queued"; delete f.result.proof;
  f.data.runs = [{ id: "automatic", activityId: "auto-run", trigger: "automatic", status: "running", startedAt: new Date().toISOString(), targets: 1, scanned: 0, cached: 0, failed: 0, deferred: 0, intelligence: "available" }];
  f.show(); await screen.findByText("Candidate check queued");
  await waitFor(() => expect(f.observers.has("auto-run")).toBe(true));
  expect(screen.queryByText(/Automatic candidate check/) !== null).toBe(true);
});
it("keeps consumer and package identity separate", async () => {
  const f = setup(); f.result.consumer.imageId = "another-image"; f.show();
  await waitFor(() => expect(f.adapter.getVulnerabilityCandidates).toHaveBeenCalled());
  await screen.findByText("Not checked yet");
  expect(screen.queryByRole("button", { name: "Update to verified image" }) === null).toBe(true);
});

it("CW-h hides candidate actions for a feature-disabled read despite an allowed descriptor", async () => {
  const f = setup();
  f.adapter.getVulnerabilityCandidates = vi.fn(async () => { throw { code: "feature-disabled" }; });
  f.show(); await screen.findByText(catalogs["en-US"].images.errors["feature-disabled"]);
  expect(screen.queryByRole("button", { name: "Check candidates" }) === null).toBe(true);
  expect(screen.queryByRole("button", { name: "Update to verified image" }) === null).toBe(true);
});
it("rechecks proof expiry when opening a confirmation after an idle period", async () => {
  const f = setup(); f.show();
  const button = await screen.findByRole("button", { name: "Update to verified image" });
  const clock = vi.spyOn(Date, "now").mockReturnValue(Date.now() + 86400001);
  try {
    await userEvent.click(button);
    expect(screen.queryByRole("alertdialog") === null).toBe(true);
    expect(f.adapter.installContainerUpdate).not.toHaveBeenCalled();
  } finally { clock.mockRestore(); }
});
it("CW-a refuses a proof that expires before confirmation execution", async () => {
  const f = setup(); f.show();
  await userEvent.click(await screen.findByRole("button", { name: "Update to verified image" }));
  const confirm = within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Confirm update" });
  const clock = vi.spyOn(Date, "now").mockReturnValue(Date.now() + 86400001);
  try {
    await userEvent.click(confirm);
    expect(f.adapter.installContainerUpdate).not.toHaveBeenCalled();
  } finally { clock.mockRestore(); }
});

it("localizes demo candidate activity through the existing EN and DE scan catalogues", async () => {
  const adapter = createDemoAdapter(), host = (await adapter.listHosts())[0]!;
  const id = await adapter.scanVulnerabilityCandidates(host.id, { advisory: "CVE-2026-1234" });
  const run = (await adapter.listActions()).find((r) => r.id === id)!;
  for (const locale of ["de", "en-US"] as const) {
    expect(renderMessage(run.title, locale)).toBe(catalogs[locale].runMessages.exact.vulnerabilityScan);
    expect(renderMessage(run.steps[0]!.label, locale)).toBe(catalogs[locale].runMessages.exact.scanImages);
  }
  expect((await adapter.getVulnerabilityCandidates(host.id, { advisory: "CVE-2026-1234" })).results).toEqual([]);
});

it.each([false, true])("SEC-4 reads baseline capability only for a real stack (stack=%s)", async (stack) => {
  const f = setup(stack); f.show();
  await screen.findByRole("button", { name: "Update to verified image" });
  await act(async () => { await Promise.resolve(); });
  const calls = vi.mocked(f.adapter.getCapability).mock.calls;
  const baseline = calls.filter(([operation]) => operation === "getUpdateBaseline");
  expect(baseline.length, "standalone containers make no baseline capability request").toBe(stack ? 1 : 0);
  if (stack) expect(baseline[0]?.[1]).toEqual({ kind: "stack", id: "example~project" });
  expect(calls.some(([, resource]) => resource?.kind === "stack" && resource.id === "example~consumer"), "a container ID is never addressed as a stack").toBe(false);
});
