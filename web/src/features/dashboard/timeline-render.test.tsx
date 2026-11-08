// Source: docklet-next/issues/360
// Revision: d4a55e15346430acc3fcf66b241684177375f39e
// Retrieved: 2026-11-08
// License: Apache-2.0
// Deviations: Original Dashboard timeline render-isolation tests through the actual events query selector.

import { act, screen } from "@testing-library/react";
import { useEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapEvent } from "@/data/http/map-dto";
import type { CodedEvent } from "@/data/message-code";
import { useEvents } from "@/data/queries";
import * as format from "@/domain/format";
import type { Host, OpsEvent } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { TimelineWidget } from "./widgets/TimelineWidget";

vi.mock("@/domain/format", async (original) => {
  const actual = await original<typeof import("@/domain/format")>();
  return { ...actual, formatTime: vi.fn(actual.formatTime) };
});

const now = Date.parse("2026-10-12T10:00:00Z");
const hosts = (name = "Example host") => [{ id: "h", name, color: "#336699", status: "online" } as Host];
const events: OpsEvent[] = Array.from({ length: 50 }, (_, i) => ({ id: `e${i}`, at: new Date(now - i * 60_000).toISOString(), kind: "restart", level: "info", title: `Event ${i}`, hostId: "h" }));
const formatted = () => vi.mocked(format.formatTime).mock.calls.length;

type Props = Parameters<typeof TimelineWidget>[0];
let show: (props: Props) => void = () => {};
function Harness(initial: Props) {
  const [props, setProps] = useState(initial);
  useEffect(() => { show = setProps; }, []);
  return <TimelineWidget {...props} />;
}

// Newest-first coded rows as the hub returns them; the query's select maps fresh objects per read.
const coded = (rows: OpsEvent[]): CodedEvent[] => rows.map((event) => mapEvent({
  id: event.id, type: "activity.message", messageCode: "activity.message", severity: "info",
  title: event.title, timestamp: event.at, createdAt: event.at, environmentId: "h",
} as Parameters<typeof mapEvent>[0]));
let refetch: () => Promise<unknown> = async () => undefined;
function QueryTimeline() {
  const query = useEvents();
  useEffect(() => { refetch = query.refetch; }, [query.refetch]);
  return query.data ? <TimelineWidget events={query.data} hosts={hosts()} now={now} /> : null;
}
async function queried(initial: OpsEvent[]) {
  let rows = coded(initial);
  const adapter = { ...createDemoAdapter(), listEvents: vi.fn(async () => structuredClone(rows)) };
  const view = renderWithApp(<QueryTimeline />, adapter);
  await view.findByRole("button", { name: /Event 0/ });
  return { view, serve: (next: OpsEvent[]) => { rows = coded(next); } };
}

afterEach(() => { act(() => setAppLocale("de")); vi.unstubAllGlobals(); });

describe("dashboard timeline", () => {
  it("does no timeline work while the Dashboard re-renders with equal hosts, and follows a host rename", () => {
    // Every marker position and the 24-hour filter construct a Date from the event time.
    const NativeDate = Date;
    const times = new Set(events.map((event) => event.at));
    let constructed = 0;
    vi.stubGlobal("Date", new Proxy(NativeDate, { construct: (target, args) => { if (times.has(args[0])) constructed++; return Reflect.construct(target, args); } }));
    renderWithApp(<Harness events={events} hosts={hosts()} now={now} />);
    const markers = formatted();
    constructed = 0;

    // Metrics and host observations give the page new, equal host arrays several times a second.
    for (let i = 0; i < 3; i++) act(() => show({ events, hosts: hosts(), now }));
    expect(constructed).toBe(0);
    expect(formatted()).toBe(markers);

    act(() => show({ events, hosts: hosts("Renamed host"), now }));
    expect(screen.getByText("Renamed host") !== null).toBe(true);
    expect(formatted()).toBe(markers);
  });

  it("builds only the new marker when a refetch through the events query prepends an event", async () => {
    setAppLocale("de");
    const { view, serve } = await queried(events);
    const before = formatted();
    serve([{ id: "new", at: new Date(now - 30_000).toISOString(), kind: "activity", level: "info", title: "Fresh event", hostId: "h" }, ...events]);
    await act(async () => { await refetch(); });
    await view.findByRole("button", { name: /Fresh event/ });
    expect(view.getAllByRole("button").length).toBe(events.length + 1);
    expect(formatted() - before).toBe(1);
  });

  it("reformats marker times when the locale changes while the events stay equal", async () => {
    setAppLocale("de");
    const { view } = await queried(events);
    act(() => setAppLocale("en-US"));
    await view.findByText("now");
    expect(view.getByRole("button", { name: /Event 0/ }).getAttribute("aria-label")).toBe(`${format.formatTime(events[0]!.at)} · Event 0`);
  });
});
