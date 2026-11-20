// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity audit view for issue 531.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import { renderRun, type CodedActionRun } from "@/data/message-code";
import { envelope, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "@/data/http/runtime-commands.test-support";
import { RuntimeControls } from "./RuntimeControls";

afterEach(cleanup);

// The sandbox producer payload: an undeterminable peer without a file stores file "".
const audit = { id: "example-audit", activityId: RUN_ID, level: "info", message: "runtime.peer-acknowledged", code: "runtime.peer-acknowledged", payload: { count: 1, peers: [{ name: "example-peer", reason: "compose.identity-unreadable", file: "" }] }, createdAt: "2026-11-20T00:00:00Z" };

it.each(["en-US", "de"] as const)("the loaded run detail shows the peer confirmation audit in %s", async (locale) => {
 const f = runtimeFixture();
 f.state.run = snapshot("direct", { type: "project_down", resourceType: "project", resourceId: "example-target", status: "success", outcomeState: "known" });
 f.state.read = () => envelope(runtimeDetail(f.state.run, { messages: [audit] as never }));
 const loaded = await f.adapter.getRuntimeRun?.("direct", RUN_ID);
 expect(loaded).toBeDefined();
 const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
 render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={client}><IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><MemoryRouter><RuntimeControls run={renderRun(loaded as CodedActionRun, locale)} /></MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
 const text = screen.getByRole("region", { name: catalogs[locale].activity.runtime.controls }).textContent ?? "";
 expect(text).toContain("example-peer");
 expect(text).toContain(locale === "de" ? "Compose-Identität" : "Compose identity");
 f.helper.reset(); client.clear();
});
