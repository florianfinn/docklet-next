// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { activityFixture, at } from "@/data/http/activity-reader.test-support";
import { mapActivitySummary } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { AdapterContext } from "@/data/use-adapter";
import { ConnectStep } from "@/features/hosts/onboarding/ConnectSteps";
import { freshDraft } from "@/features/hosts/onboarding/onboarding-draft";
import { ActionToastView } from "./ActionToastView";

it("onboarding renders a cold list coded step without reading detail", async () => {
  const f = activityFixture();
  f.state.lists.good![0]!.stepTrace = [{ id: "step-connect", name: "connect", code: "activity.message", status: "running", startedAt: at }];
  await f.adapter.listActions();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const draft = freshDraft({ connection: "direct", projectDir: "/srv/example", installDir: "/srv/example/agent", puid: "", pgid: "", detectSocketGroup: true }, []);
  draft.runId = "run-good";
  const view = render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={client}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter>
    <ConnectStep draft={draft} host={undefined} hubUrl="https://hub.example.com" />
  </MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  try {
    expect(await screen.findByText("connect")).toBeDefined();
    expect(f.requests.filter((path) => /\/activities\/|\/runtime-runs\//.test(path))).toEqual([]);
  } finally { view.unmount(); client.clear(); }
});

it("an older agent's omitted trace displays unknown progress instead of complete empty history", () => {
  const f = activityFixture(); const run = renderRun(mapActivitySummary(f.state.lists.good![0]!), "en-US");
  const view = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter>
    <ActionToastView run={run} design="bar" detail="full" now={Date.parse(at)} major={false} onClose={() => {}} onOpen={() => {}} />
  </MemoryRouter></IntlProvider>);
  try { expect(view.container.querySelector('[data-slot="toast-meta"]')?.textContent).toContain("–"); expect(view.container.textContent).not.toContain("0/0"); }
  finally { view.unmount(); }
});
