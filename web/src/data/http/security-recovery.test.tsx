// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: b6e3b7c1952172431c244c2a4a7dffdc6c3103f9
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original held-host recovery regressions for issue #263.

import { act, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { securityRecoveryInterval, SECURITY_RECOVERY_MS } from "@/domain/security";
import SecurityPage from "@/features/security/SecurityPage";
import { renderWithApp } from "@/test/render";
import { securityFixture } from "./vulnerabilities.test-support";

afterEach(() => { cleanup(); vi.useRealTimers(); setAppLocale("de"); });

it("re-reads a transiently unavailable host on the mounted page and stops after recovery", async () => {
  vi.useFakeTimers(); setAppLocale("en-US");
  const f = securityFixture(); f.state.failureHost = "second";
  const reads = () => f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/environments/second/vulnerabilities/runs")).length;
  renderWithApp(<SecurityPage />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(1_000));
  expect(document.body.textContent).toContain("Scan data unavailable for second");
  expect(document.body.textContent).toContain("CVE-2026-0001");
  f.state.failureHost = "";
  await act(() => vi.advanceTimersByTimeAsync(SECURITY_RECOVERY_MS + 1_000));
  expect(document.body.textContent).not.toContain("Scan data unavailable for second");
  const settled = reads();
  await act(() => vi.advanceTimersByTimeAsync(3 * SECURITY_RECOVERY_MS));
  expect(reads()).toBe(settled);
});

it("recovers a transport failure of the read-only summaries request on the mounted page", async () => {
  vi.useFakeTimers(); setAppLocale("en-US");
  const f = securityFixture(), original = f.fetcher.getMockImplementation()!;
  let broken = true;
  f.fetcher.mockImplementation(async (input, init) => {
    if (broken && String(input).endsWith("/environments/second/images/vulnerabilities/summaries")) throw new TypeError("network failure");
    return original(input, init);
  });
  renderWithApp(<SecurityPage />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(1_000));
  expect(document.body.textContent).toContain("Scan data unavailable for second");
  broken = false;
  await act(() => vi.advanceTimersByTimeAsync(SECURITY_RECOVERY_MS + 1_000));
  expect(document.body.textContent).not.toContain("Scan data unavailable for second");
  expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/vulnerabilities/scan"))).toBe(false);
});

// A transport loss at fetch or while streaming the body is recoverable; a refusal or malformed body is not.
const lostBody = () => new Response(new ReadableStream({ start(controller) { controller.error(new TypeError("stream lost")); } }), { status: 200, headers: { "Content-Type": "application/json" } });
const hostCapability = (url: URL) => url.pathname === "/api/capabilities" && url.searchParams.get("kind") === "host" && url.searchParams.get("id") === "second";
type Fault = (url: URL) => Response | undefined;
const recoverable: [string, Fault][] = [
  ["host capability fetch", (url) => { if (hostCapability(url)) throw new TypeError("network failure"); return undefined; }],
  ["host capability body", (url) => hostCapability(url) ? lostBody() : undefined],
  ...["images", "projects", "containers"].map((kind): [string, Fault] => [`${kind} body`, (url) => url.pathname.endsWith(`/environments/second/${kind}`) ? lostBody() : undefined]),
];
const permanent: [string, Fault][] = [
  ["denied host descriptor", (url) => hostCapability(url) ? Response.json({ success: true, data: [{ operationId: "getSecurity", available: false, disabledReasonCode: "permission.denied", resourceRef: { kind: "host", id: "second" } }] }) : undefined],
  ["malformed images body", (url) => url.pathname.endsWith("/environments/second/images") ? new Response("{", { status: 200, headers: { "Content-Type": "application/json" } }) : undefined],
];
function faulted(fault: Fault) {
  const f = securityFixture(), original = f.fetcher.getMockImplementation()!;
  const state = { broken: true };
  f.fetcher.mockImplementation(async (input, init) => {
    const response = state.broken ? fault(new URL(String(input), "https://hub.example.com")) : undefined;
    return response ?? original(input, init);
  });
  // Every security read starts with the hub settings read; capability descriptors are cached.
  return { f, state, reads: () => f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/environments/0/settings/public")).length };
}
it.each(recoverable)("recovers a lost %s on the mounted page", async (_, fault) => {
  vi.useFakeTimers(); setAppLocale("en-US");
  const { f, state } = faulted(fault);
  renderWithApp(<SecurityPage />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(1_000));
  expect(document.body.textContent).toContain("Scan data unavailable for second");
  state.broken = false;
  await act(() => vi.advanceTimersByTimeAsync(SECURITY_RECOVERY_MS + 1_000));
  expect(document.body.textContent).not.toContain("Scan data unavailable for second");
});
it.each(permanent)("keeps a %s unavailable without re-reading", async (_, fault) => {
  vi.useFakeTimers(); setAppLocale("en-US");
  const { f, reads } = faulted(fault);
  renderWithApp(<SecurityPage />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(1_000));
  expect(document.body.textContent).toContain("Scan data unavailable for second");
  const before = reads();
  await act(() => vi.advanceTimersByTimeAsync(3 * SECURITY_RECOVERY_MS));
  expect(document.body.textContent).toContain("Scan data unavailable for second");
  expect(reads()).toBe(before);
});

it("re-reads only for host failures that can recover without operator action", () => {
  expect(securityRecoveryInterval({ unavailable: [{ code: "agent.unavailable" }] })).toBe(SECURITY_RECOVERY_MS);
  expect(securityRecoveryInterval({ unavailable: [{ code: "vulnerability.read-timeout" }] })).toBe(SECURITY_RECOVERY_MS);
  expect(securityRecoveryInterval({ unavailable: [{ code: "vulnerability.host-incompatible" }] })).toBe(false);
  expect(securityRecoveryInterval({ unavailable: [] })).toBe(false);
  expect(securityRecoveryInterval(undefined)).toBe(false);
});

it("judges a host by its fresh security batch rather than an older stale host snapshot", async () => {
  const f = securityFixture();
  await f.adapter.getCurrentUser();
  f.inventory.state.reason = "agent.unavailable";
  await f.adapter.listImages();
  expect((await f.adapter.listHosts()).every((host) => host.inventory?.stale === true)).toBe(true);
  f.inventory.state.reason = "";
  const overview = await f.adapter.getSecurity();
  expect(overview.unavailable).toEqual([]);
  expect(overview.images.some((image) => image.unavailable)).toBe(false);
});

it("keeps a host with a stale empty inventory unavailable", async () => {
  const f = securityFixture();
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => {
    const response = await original(input, init);
    if (!new URL(String(input), "https://hub.example.com").pathname.endsWith("/second/images")) return response;
    const body = await response.json();
    return Response.json({ ...body, data: [], pagination: { totalItems: 0 } }, { headers: response.headers });
  });
  await f.adapter.getCurrentUser();
  f.inventory.state.reason = "agent.unavailable";
  const overview = await f.adapter.getSecurity();
  expect(overview.images.some((image) => image.hostId === "second")).toBe(false);
  expect(overview.unavailable?.some((entry) => entry.hostId === "second")).toBe(true);
});
