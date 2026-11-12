// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that a truncated event window keeps its full-history link when the shown hosts have no rows (R1D-7).

import { screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import type { Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { EventsWidget } from "./widgets/SmallWidgets";
import { TimelineWidget } from "./widgets/TimelineWidget";

afterEach(() => { setAppLocale("de"); });
const otherHostEvents = Array.from({ length: 500 }, (_, i) => ({ id: `e${i}`, hostId: "a", at: "2026-11-11T10:00:00Z", title: "Example", kind: "restart" as const, level: "info" as const }));

it.each([["en-US", "Show all events"], ["de", "Alle Ereignisse anzeigen"]] as const)("R1D-7 a scoped timeline whose bounded window holds only other hosts keeps the full-history link (%s)", (locale, link) => {
  setAppLocale(locale);
  renderWithApp(<TimelineWidget events={otherHostEvents} hosts={[{ id: "b", name: "B", status: "online", color: "#336699" } as Host & { color: string }]} now={Date.parse("2026-11-11T11:00:00Z")} total={501} />);
  expect(screen.queryByRole("button", { name: link }) !== null).toBe(true);
});

it.each([["en-US", "Show all events", 501, true], ["en-US", "Show all events", 500, false]] as const)("R1D-7 an empty scoped events list shows the link only when the window was truncated (%s, %i)", (locale, link, total, shown) => {
  setAppLocale(locale);
  renderWithApp(<EventsWidget events={[]} hosts={new Map()} total={total} />);
  expect(screen.queryByRole("button", { name: link }) !== null).toBe(shown);
});
