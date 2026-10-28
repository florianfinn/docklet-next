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
