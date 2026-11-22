// Source: docklet-next/issues/572
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Edge hub address guidance and effect-synchronised acceptance coverage.; M36 #572: Edge hub address guidance.

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { HubDiscovery } from "@/domain/hub-discovery";
import { renderWithApp } from "@/test/render";
import { GeneralFields } from "./GeneralSection";
import { hubAddressPortWarning } from "./hub-address-guidance";

const discovery: HubDiscovery = { detectedAt: "2026-11-22T12:00:00Z", simulated: false, interfaces: [], hostAddresses: [], container: { name: "hub", networkMode: "bridge", listenPort: 3552, publishedPorts: [{ containerPort: 3552, hostPort: 8080 }] }, candidates: [{ id: "ip", url: "http://192.0.2.10:8080", source: "published", kind: "ip", reach: "unknown" }] };
afterEach(() => { cleanup(); setAppLocale("de"); });

it.each([
  ["192.0.2.10", discovery, 8080],
  ["192.0.2.10:3552", discovery, undefined],
  ["[2001:db8::10]:3552", discovery, undefined],
  ["2001:db8::10", discovery, 8080],
  ["", discovery, undefined],
  ["192.0.2.10", undefined, undefined],
  ["192.0.2.10", { ...discovery, container: { ...discovery.container!, publishedPorts: [{ containerPort: 3552, hostPort: 80 }, { containerPort: 9000, hostPort: 9001 }] } }, undefined],
  // Published on 80 and another port: the portless address is valid, as in the onboarding default.
  ["192.0.2.10", { ...discovery, container: { ...discovery.container!, publishedPorts: [{ containerPort: 3552, hostPort: 8080 }, { containerPort: 3552, hostPort: 80 }] } }, undefined],
] satisfies [string, HubDiscovery | undefined, number | undefined][])("warns only for a missing port with a relevant published binding: %s", (address, found, want) => {
  expect(hubAddressPortWarning(address, found)).toBe(want);
});

it("offers discovered IP addresses as one-click field replacements and warns about the saved address", async () => {
  setAppLocale("en-US");
  const onChange = vi.fn();
  renderWithApp(<GeneralFields value={{ hubName: "Example", hubAddress: "192.0.2.10:3552", domainUrl: "", linkBase: "auto", timeZone: "UTC" }} savedAddress="192.0.2.10" discovery={discovery} onChange={onChange} showErrors={false} />);
  expect(screen.getByText(/The saved address has no port/) !== null).toBe(true);
  expect(screen.getByText(/Direct agents do not need it/) !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "192.0.2.10:8080" }));
  expect(onChange).toHaveBeenCalledWith({ hubAddress: "192.0.2.10:8080" });
});
