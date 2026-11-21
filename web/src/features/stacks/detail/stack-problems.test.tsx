// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original per-stack load problem and Compose name peer rendering for issues 547 and 548.

import { screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderStackDetail } from "@/data/message-code";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapStack, type ProjectRow } from "@/data/http/inventory-mapping";
import { renderWithApp } from "@/test/render";
import { StackHeader } from "./StackHeader";

afterEach(() => setAppLocale("de"));

const base = { id: "media", name: "Media", status: "running", definitionHint: "definition-invalid", runtimeServices: [] } as unknown as ProjectRow;

it.each([
  ["de", { definitionReason: "compose.variable-missing", definitionVariable: "APP_TAG" }, "Die erforderliche Variable APP_TAG hat keinen Wert. Setze sie in der .env des Stacks oder in der env-Datei, mit der der Stack gestartet wurde."],
  ["en-US", { definitionReason: "compose.variable-missing", definitionVariable: "APP_TAG" }, "Required variable APP_TAG has no value. Set it in the stack's .env file or in the env file the stack was started with."],
  ["en-US", { definitionReason: "compose.variable-missing", definitionVariable: "bad name" }, "A required variable has no value."],
  ["de", { definitionReason: "compose.env-label-conflict" }, "Die Container dieses Stacks vermerken unterschiedliche env-Dateien."],
  ["en-US", { definitionReason: "compose.env-label-invalid" }, "The env file recorded on this stack's containers is outside the projects directory or cannot be read safely."],
  ["en-US", { definitionReason: "compose.future-reason" }, "The Compose definition cannot be resolved. Repair it to restore the services list."],
] as const)("%s renders the stack's own load problem %j", async (locale, fields, text) => {
  setAppLocale(locale);
  const adapter = createDemoAdapter();
  const original = renderStackDetail(await adapter.getStack("media"), locale);
  const mapped = mapStack({ ...base, ...fields } as unknown as ProjectRow, "0", [], { stale: false });
  renderWithApp(<StackHeader stack={{ ...original, ...mapped }} host={undefined} />, adapter);
  expect(screen.getByText(text) !== null).toBe(true);
});

it("warns about stacks sharing the Compose name without blocking anything", async () => {
  const adapter = createDemoAdapter();
  const original = renderStackDetail(await adapter.getStack("media"), "de");
  const peers = mapStack({ ...base, definitionHint: undefined, composeNamePeers: ["beta", "gamma"] } as unknown as ProjectRow, "0", [], { stale: false });
  const view = renderWithApp(<StackHeader stack={{ ...original, ...peers }} host={undefined} />, adapter);
  expect(screen.getByText("Die Stacks beta, gamma verwenden denselben Compose-Projektnamen. Aktionen an diesem Stack laufen trotzdem.") !== null).toBe(true);
  view.unmount();
  const none = mapStack({ ...base, definitionHint: undefined, composeNamePeers: [] } as unknown as ProjectRow, "0", [], { stale: false });
  expect(none.composeNamePeers).toBeUndefined();
  const bounded = mapStack({ ...base, composeNamePeers: Array.from({ length: 20 }, (_, i) => `peer-${i}`) } as unknown as ProjectRow, "0", [], { stale: false });
  expect(bounded.composeNamePeers?.length).toBe(16);
});
