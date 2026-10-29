// Source: docklet-next/issues/284
// Revision: f8d52fd7e76e5dc1ca650e35e1aacc02fe8bc1af
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original guard for the Deployments-tab volume backup hint.

import { screen } from "@testing-library/react";
import { expect, it } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { getAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderStackDetail } from "@/data/message-code";
import { renderWithApp } from "@/test/render";

import { DeploymentsTab } from "./DeploymentsTab";

it("points from deployment restore points to volume backups", async () => {
  const adapter = createDemoAdapter();
  renderWithApp(<DeploymentsTab stack={renderStackDetail(await adapter.getStack("craft"), "de")} />, adapter);
  const backups = catalogs[getAppLocale()].volumes.backups;
  expect(document.body.textContent?.includes(backups.deploymentHint)).toBe(true);
  expect(screen.getByRole("link", { name: backups.title }).getAttribute("href")).toBe("/backups");
});
