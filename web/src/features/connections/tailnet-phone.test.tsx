// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { mapHost } from "@/data/http/hosts";
import { renderRoutes, renderWithApp } from "@/test/render";
import SetupPage from "@/features/setup/SetupPage";
import { patchSetup, restartSetup } from "@/features/setup/setup-draft";
import OnboardingPage from "@/features/hosts/onboarding/OnboardingPage";
import { onboardingStore } from "@/features/hosts/onboarding/onboarding-store";
import { ConnectionsSection } from "./ConnectionsSection";
import { TailnetHostCause } from "./TailnetHosts";
import { connectedStatus, peerHost, tailnetFixture } from "./tailnet.test-support";

type HappyWindow = Window & { happyDOM: { setViewport: (value: { width: number; height: number }) => void } };
const viewport = (width: number) => (window as unknown as HappyWindow).happyDOM.setViewport({ width, height: 844 });
beforeEach(() => { setAppLocale("en-US"); viewport(390); });
afterEach(() => { setAppLocale("de"); restartSetup(); onboardingStore.set(null); viewport(1024); });

function phoneBounds(root: HTMLElement) {
  // happy-dom does not lay out Tailwind. Check its measurable widths plus the shrink/wrap contracts, as settings-nav does.
  for (const node of [root, ...root.querySelectorAll<HTMLElement>("*")]) {
    expect(node.getBoundingClientRect().width).toBeLessThanOrEqual(390);
    expect(node.scrollWidth).toBeLessThanOrEqual(390);
    expect(Number.parseFloat(node.style.minWidth) || 0).toBeLessThanOrEqual(390);
  }
}

it("contains the shared setup form and policy at 390 px", async () => {
  const f = tailnetFixture({ ...connectedStatus(), enabled: false, connection: { state: "off" } });
  f.state.settings.policySnippet = '{"example": "' + "tag:docklet-agent".repeat(30) + '"}';
  patchSetup({ reached: 4 });
  const view = renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup?step=tailnet"], f.adapter);
  await screen.findByRole("button", { name: "Copy policy" });
  const form = view.container.querySelector<HTMLElement>('[data-slot="tailnet-connection"]')!;
  expect(form.classList.contains("min-w-0")).toBe(true);
  expect(form.querySelector("pre")?.classList.contains("whitespace-pre-wrap")).toBe(true);
  expect(form.querySelector("pre")?.classList.contains("break-all")).toBe(true);
  phoneBounds(view.container);
});

it("contains Connections at 390 px", async () => {
  const f = tailnetFixture(); const view = renderWithApp(<ConnectionsSection />, f.adapter);
  await screen.findByRole("button", { name: "Credential and policy" });
  expect(view.container.querySelector("section")?.classList.contains("min-w-0")).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Credential and policy" }));
  expect(view.container.querySelector("pre")?.classList.contains("whitespace-pre-wrap")).toBe(true);
  phoneBounds(view.container);
});

it("contains the stacked route cards at 390 px", async () => {
  const f = tailnetFixture(); const view = renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }], ["/hosts/new"], f.adapter);
  const card = await screen.findByRole("radio", { name: /Tailscale Private/ });
  await waitFor(() => expect((card as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(card);
  expect(view.container.querySelector('[data-slot="tailnet-route"]')?.classList.contains("min-w-0")).toBe(true);
  expect(screen.getByRole("radiogroup", { name: "Route to the hub" }).classList.contains("grid")).toBe(true);
  phoneBounds(view.container);
});

it("contains the host chain dialog at 390 px and keeps later links neutral", async () => {
  const f = tailnetFixture(); const host = mapHost({ ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "offline", failingLink: "peer" } });
  renderWithApp(<TailnetHostCause host={host} />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Show cause" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.classList.contains("max-w-[calc(100vw-2rem)]")).toBe(true);
  expect(dialog.querySelector('[data-link="tunnel"]')?.classList.contains("text-muted-foreground")).toBe(true);
  phoneBounds(dialog);
});
