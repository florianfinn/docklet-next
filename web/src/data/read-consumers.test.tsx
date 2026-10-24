// Source: docklet-next/issues/211
// Revision: 31a3237938f4b560d550c9f0a34ebc547fddde0d
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original actual-provider observer demand regression with retained query cache.

import { act, render } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { expect, it, vi } from "vitest";
import { AdapterProvider } from "./AdapterProvider";
import { useEvents } from "./queries";
import { eventDemandFixture } from "./http/event-demand.test-support";

it("actual ChangeBridge counts mounted event observers rather than cached history", async () => {
  const f = eventDemandFixture(); await f.adapter.getCurrentUser();
  const observed = vi.fn();
  function Events() { observed(useEvents().data); return null; }
  const tree = (mounted: boolean) => <AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={{}}>{mounted ? <Events /> : null}</IntlProvider></AdapterProvider>;
  vi.useFakeTimers();
  const drain = () => act(async () => { await vi.advanceTimersByTimeAsync(0); });
  const view = render(tree(false));
  try {
    await drain(); f.source.open(); await drain(); expect(f.calls).toHaveLength(0);
    view.rerender(tree(true)); await drain(); expect(f.calls).toHaveLength(8);
    f.notify(); await drain(); expect(f.calls).toHaveLength(16);
    view.rerender(tree(false)); await drain();
    for (let i = 0; i < 5; i++) { f.notify(); await drain(); }
    expect(f.calls).toHaveLength(16);
    f.state.rows[0] = { ...f.state.rows[0]!, id: "after-remount" };
    view.rerender(tree(true)); await drain();
    expect(f.calls).toHaveLength(24);
    expect(observed.mock.lastCall?.[0]?.[0]?.id).toBe("after-remount");
  } finally { view.unmount(); vi.useRealTimers(); }
});
