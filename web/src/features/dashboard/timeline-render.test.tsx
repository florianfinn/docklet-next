// Source: docklet-next/issues/360
// Revision: d4a55e15346430acc3fcf66b241684177375f39e
// Retrieved: 2026-11-08
// License: Apache-2.0
// Deviations: Original Dashboard timeline render-isolation tests.

import { act, screen } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

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

type Props = Parameters<typeof TimelineWidget>[0];
let show: (props: Props) => void = () => {};
function Harness(initial: Props) {
  const [props, setProps] = useState(initial);
  useEffect(() => { show = setProps; }, []);
  return <TimelineWidget {...props} />;
}

describe("dashboard timeline", () => {
  it("renders its markers once while the Dashboard re-renders with equal hosts, and follows real changes", () => {
    renderWithApp(<Harness events={events} hosts={hosts()} now={now} />);
    const formatted = vi.mocked(format.formatTime);
    const initial = formatted.mock.calls.length;
    expect(initial >= events.length).toBe(true);

    // Metrics and host observations give the page new, equal host arrays several times a second.
    for (let i = 0; i < 3; i++) act(() => show({ events, hosts: hosts(), now }));
    expect(formatted.mock.calls.length).toBe(initial);

    // A refetch keeps unchanged events identical; only the new marker is built.
    const added: OpsEvent = { id: "new", at: new Date(now - 30_000).toISOString(), kind: "deploy", level: "info", title: "Fresh event", hostId: "h" };
    act(() => show({ events: [added, ...events], hosts: hosts("Renamed host"), now }));
    expect(formatted.mock.calls.length).toBe(initial + 1);
    expect(screen.getByText("Renamed host") !== null).toBe(true);
    expect(screen.getAllByRole("button").length).toBe(events.length + 1);
    expect(screen.getByRole("button", { name: /Fresh event/ }) !== null).toBe(true);
  });
});
