// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original detail gestures, plain-text safety and selection lifetime regressions. M21 #437: advisory detail.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { advisoryKey, type AdvisoryRead } from "@/data/vulnerability-advisory";
import { decodeAdvisory } from "@/data/http/vulnerability-advisory";
import fixture from "@/data/http/testdata/vulnerability-advisory.json";
import { renderRoutes, renderWithApp } from "@/test/render";
import { AdvisoryContent } from "./AdvisoryContent";
import SecurityPage from "./SecurityPage";
import VulnerabilityPage from "./VulnerabilityPage";
import { useAdvisoryDetail } from "./use-advisory-detail";
import type { Topic } from "@/data/adapter";

vi.mock("@/shared/hooks/use-fit-view", () => ({ useFitView: () => ({ view: "table", ref: () => {} }) }));
afterEach(() => { setAppLocale("de"); localStorage.clear(); });
const key = advisoryKey("CVE-2099-0001");
const read = (value = key): AdvisoryRead => ({ key: value, hosts: [{ hostId: "example", detail: decodeAdvisory(fixture, "CVE-2099-0001") }] });

it("keeps hostile metadata plain and links only HTTP references with external protections", () => {
  setAppLocale("en-US");
  const { container } = renderWithApp(<AdvisoryContent data={read()} hosts={new Map()} onRetry={() => {}} />);
  expect(container.querySelector("script") === null).toBe(true);
  expect(screen.queryByText("<script>example</script>") !== null).toBe(true);
  expect(screen.queryByRole("link", { name: "javascript:example()" }) === null).toBe(true);
  const link = screen.getByRole("link", { name: "https://example.com/advisory" });
  expect(link.getAttribute("rel")).toBe("noopener noreferrer"); expect(link.querySelector("svg") !== null).toBe(true);
});

it("never describes unknown host or failed Docker usage as no affected containers", () => {
  setAppLocale("en-US");
  const r = read(); r.hosts[0]!.detail!.usage = { state: "unavailable", code: "image.usage-unavailable" };
  r.hosts[0]!.detail!.occurrences.forEach((o) => { o.containers = []; o.updates = []; }); r.hosts.push({ hostId: "offline", code: "vulnerability.host-offline" });
  renderWithApp(<AdvisoryContent data={r} hosts={new Map()} onRetry={() => {}} />);
  expect(screen.queryAllByText(/Container usage is unavailable/).length).toBeGreaterThan(0);
  expect(screen.queryByText(/could not be read and are unknown/) !== null).toBe(true);
  expect(screen.queryByText(/No affected containers/) === null).toBe(true);
});

it("opens by row click and Enter, keeps filters and scroll mounted, and restores row focus", async () => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter(), detail = read();
  adapter.getVulnerabilityAdvisory = vi.fn(async () => detail);
  adapter.listHosts = async () => [];
  adapter.listContainers = async () => []; adapter.listStacks = async () => [];
  adapter.getSecurity = async () => ({ enabled: true, schedule: { mode: "manual" }, history: [], images: [{ ref: "example.com/app:1", hostId: "example", usage: "running", counts: { critical: 0, high: 2, medium: 0, low: 0 }, scannedAt: "2026-11-09T12:00:00Z" }], findings: fixture.occurrences.map((o, i) => ({ id: o.vulnerabilityId, pkg: o.pkgName, imageId: "image", image: "example.com/app:1", installed: "1", hostId: "example", severity: "high", cvss: 8, title: `${i}` })) });
  const { router } = renderRoutes([{ path: "/security", element: <SecurityPage /> }, { path: "/security/vulnerabilities/:key", element: <VulnerabilityPage /> }], ["/security?tab=findings"], adapter);
  const table = await screen.findByRole("table", { name: "Vulnerabilities" });
  const filter = within(table).getByRole("searchbox", { name: /Filter.*Package/i });
  await userEvent.type(filter, "alpha"); table.parentElement!.scrollTop = 70;
  const row = table.querySelector<HTMLElement>("tbody [data-security-row]")!;
  await userEvent.click(row);
  await screen.findByRole("dialog");
  expect(router.state.location.search).toContain(`finding=${key}`);
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  expect(document.activeElement === row).toBe(true); expect((filter as HTMLInputElement).value).toBe("alpha"); expect(table.parentElement!.scrollTop).toBe(70);
  await userEvent.keyboard("{Enter}"); await screen.findByRole("dialog");
  const pageLink = screen.getByRole("link", { name: "Open dedicated page" }); expect(pageLink.getAttribute("href")).toContain(`/security/vulnerabilities/${key}`);
  await userEvent.click(pageLink); await screen.findByText("Scope: all permitted hosts. Unreachable hosts remain unknown.");
  expect(router.state.location.pathname).toBe(`/security/vulnerabilities/${key}`);
});

it("reloads an advisory URL after rescan and restores focus to the first matching row", async () => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter();
  adapter.getVulnerabilityAdvisory = vi.fn(async () => read());
  const { router } = renderRoutes([{ path: "/security", element: <SecurityPage /> }, { path: "/security/vulnerabilities/:key", element: <VulnerabilityPage /> }], [`/security?tab=findings&finding=${key}`], adapter);
  await screen.findByRole("dialog"); await screen.findByText("Example advisory");
  expect(adapter.getVulnerabilityAdvisory).toHaveBeenCalledWith(key, expect.any(Array), expect.any(AbortSignal));
  await act(async () => { await router.navigate(`/security/vulnerabilities/${key}`); });
  await screen.findByText("Example advisory");
  expect(adapter.getVulnerabilityAdvisory).toHaveBeenLastCalledWith(key, [], expect.any(AbortSignal));
});

it.each([true, false])("restores URL-driven sheet focus with a matching row: %s", async (matching) => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter(), overview = await adapter.getSecurity();
  const advisory = matching ? overview.findings[0]!.id : "CVE-2099-0001", urlKey = advisoryKey(advisory);
  adapter.getVulnerabilityAdvisory = async () => {
    const result = read(urlKey), detail = result.hosts[0]!.detail!;
    detail.advisory!.vulnerabilityId = advisory;
    detail.occurrences.forEach((o) => { o.vulnerabilityId = advisory; });
    return result;
  };
  renderRoutes([{ path: "/security", element: <SecurityPage /> }], [`/security?finding=${urlKey}`], adapter);
  await screen.findByRole("dialog");
  await waitFor(() => expect(document.querySelector("[data-slot=security-table]") !== null).toBe(true));
  const target = matching ? document.querySelector<HTMLElement>(`[data-advisory-key="${urlKey}"]`) : document.querySelector<HTMLElement>("[data-slot=security-table]");
  expect(target !== null).toBe(true);
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  expect(document.activeElement === target).toBe(true);
});

it("keeps a typed ignore reason and its focus across change notifications", async () => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter(), listeners = new Set<(topics: Topic[]) => void>();
  adapter.subscribeChanges = (listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
  adapter.listHosts = async () => [];
  adapter.getCapability = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
  let release!: () => void;
  adapter.getVulnerabilityAdvisory = vi.fn(async () => { if (vi.mocked(adapter.getVulnerabilityAdvisory).mock.calls.length > 1) await new Promise<void>((resolve) => { release = resolve; }); return read(); });
  renderRoutes([{ path: "/security/vulnerabilities/:key", element: <VulnerabilityPage /> }], [`/security/vulnerabilities/${key}`], adapter);
  const input = (await screen.findAllByLabelText("Reason (optional)"))[0]!;
  await userEvent.type(input, "accepted risk");
  await act(async () => { listeners.forEach((fn) => fn(["security"])); listeners.forEach((fn) => fn(["hosts"])); });
  await waitFor(() => expect(adapter.getVulnerabilityAdvisory).toHaveBeenCalledTimes(2));
  expect([(screen.getAllByLabelText("Reason (optional)")[0] as HTMLInputElement).value, document.activeElement === input]).toEqual(["accepted risk", true]);
  await act(async () => { release(); });
  expect([(screen.getAllByLabelText("Reason (optional)")[0] as HTMLInputElement).value, document.activeElement === input]).toEqual(["accepted risk", true]);
});

it("picks the newest modification instant across hosts, not the newest date string", () => {
  setAppLocale("en-US");
  const earlier = read(), later = read();
  earlier.hosts[0]!.detail!.advisory!.lastModifiedDate = "2026-11-09T13:30:00+02:00";
  later.hosts[0]!.detail!.advisory!.lastModifiedDate = "2026-11-09T12:00:00Z";
  later.hosts[0]!.detail!.advisory!.sourceOccurrence = { ...later.hosts[0]!.detail!.advisory!.sourceOccurrence, package: "newest-package" };
  const data: AdvisoryRead = { key, hosts: [{ ...earlier.hosts[0]!, hostId: "first" }, { ...later.hosts[0]!, hostId: "second" }] };
  renderWithApp(<AdvisoryContent data={data} hosts={new Map()} onRetry={() => {}} />);
  expect(screen.queryByText(/newest-package/) !== null).toBe(true);
});

it("ignores one occurrence through the detail action and reloads both detail and active table", async () => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter(), listeners = new Set<(topics: Topic[]) => void>();
  let current = read();
  adapter.subscribeChanges = (listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
  adapter.listContainers = async () => []; adapter.listStacks = async () => []; adapter.listHosts = async () => [];
  adapter.getCapability = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
  adapter.getVulnerabilityAdvisory = vi.fn(async () => structuredClone(current));
  adapter.getSecurity = async () => ({ enabled: true, schedule: { mode: "manual" }, history: [], images: [{ ref: "example.com/app:1", hostId: "example", usage: "running", counts: { critical: 0, high: current.hosts[0]!.detail!.occurrences.filter((o) => !o.ignored).length, medium: 0, low: 0 }, scannedAt: "2026-11-09T12:00:00Z" }], findings: current.hosts[0]!.detail!.occurrences.filter((o) => !o.ignored).map((o) => ({ id: o.vulnerabilityId, pkg: o.pkgName, imageId: o.imageId, image: "example.com/app:1", installed: "1", hostId: "example", severity: "high", cvss: 8, title: "Example" })) });
  adapter.ignoreVulnerability = vi.fn(async (_host, _advisory, occurrence) => { current = structuredClone(current); const target = current.hosts[0]!.detail!.occurrences.find((o) => o.pkgName === occurrence.pkgName)!; target.ignored = true; target.ignoreId = "new-ignore"; listeners.forEach((fn) => fn(["security"])); });
  const { router } = renderRoutes([{ path: "/security", element: <SecurityPage /> }, { path: "/security/vulnerabilities/:key", element: <VulnerabilityPage /> }], ["/security?tab=findings"], adapter);
  const table = await screen.findByRole("table", { name: "Vulnerabilities" });
  expect(table.querySelectorAll("[data-security-row]").length).toBe(1);
  await userEvent.click(table.querySelector<HTMLElement>("[data-security-row]")!);
  const dialog = await screen.findByRole("dialog");
  await userEvent.click(await within(dialog).findByRole("button", { name: "Ignore this occurrence" }));
  await waitFor(() => expect(adapter.ignoreVulnerability).toHaveBeenCalledTimes(1));
  expect(vi.mocked(adapter.ignoreVulnerability).mock.calls[0]?.[2].pkgName).toBe("beta");
  await waitFor(() => expect(within(dialog).queryAllByRole("button", { name: "Stop ignoring this occurrence" }).length).toBe(2));
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(table.querySelectorAll("[data-security-row]").length).toBe(0));
  await act(async () => { await router.navigate(`/security/vulnerabilities/${key}`); });
  await waitFor(() => expect(screen.queryAllByRole("button", { name: "Stop ignoring this occurrence" }).length).toBe(2));
});

it.each(["advisory", "scope", "close", "logout"])("discards late detail A after a %s change", async (change) => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter();
  let resolveA!: (r: AdvisoryRead) => void;
  const held = new Promise<AdvisoryRead>((resolve) => { resolveA = resolve; });
  adapter.getVulnerabilityAdvisory = vi.fn(async (value, ids) => value === "A" && ids?.[0] === "first" ? held : { key: value, hosts: [] });
  let version = 0, active = true; const listeners = new Set<() => void>();
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => active, listen: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; } });
  function Probe() {
    const [selection, setSelection] = useState<string | null>("A"), [scope, setScope] = useState(["first"]);
    const result = useAdvisoryDetail(selection, scope);
    return <><button onClick={() => setSelection("B")}>select</button><button onClick={() => setScope(["second"])}>scope</button><button onClick={() => setSelection(null)}>close</button><p data-testid="result">{result.data?.key ?? "empty"}</p></>;
  }
  renderWithApp(<Probe />, adapter);
  await waitFor(() => expect(adapter.getVulnerabilityAdvisory).toHaveBeenCalledWith("A", ["first"], expect.any(AbortSignal)));
  if (change === "logout") act(() => { version++; active = false; listeners.forEach((fn) => fn()); });
  else await userEvent.click(screen.getByRole("button", { name: change === "advisory" ? "select" : change }));
  if (change !== "logout" && change !== "close") await waitFor(() => expect(screen.getByTestId("result").textContent).toBe(change === "advisory" ? "B" : "A"));
  await act(async () => { resolveA({ key: "OLD", hosts: [] }); await held; });
  expect(screen.getByTestId("result").textContent).not.toBe("OLD");
});
