// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original fleet ring condition feed test from the hub event shape, issue #371 (FR5).

import { screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapEvent } from "@/data/http/map-dto";
import type { components } from "@/api/generated/schema";
import { renderWithApp } from "@/test/render";

import { ConditionFeed } from "./ConditionFeed";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => setAppLocale("de"));

type Event = components["schemas"]["EventEvent"];
const event = (id: string, code: string, metadata: Record<string, unknown>, minutesAgo: number): Event => ({
  id, type: code, messageCode: code, severity: "warning", title: code, description: code, resourceType: "container", resourceId: id, resourceName: "web",
  environmentId: "host", metadata, timestamp: new Date(Date.now() - minutesAgo * 60_000).toISOString(),
} as unknown as Event);

it("lists the last five condition events with container, exit code, restarts and relative time, without other events", async () => {
  const wire = [
    event("e1", "container.condition.down", { hostId: "host", containerName: "web", exitCode: 137, restarts: 0 }, 1),
    event("e2", "stack.deployed", {}, 2),
    event("e3", "container.condition.restart-loop", { hostId: "host", containerName: "db", exitCode: 0, restarts: 4 }, 3),
    ...["e4", "e5", "e6", "e7"].map((id, i) => event(id, "container.condition.running-again", { hostId: "host", containerName: id, exitCode: 0, restarts: 0 }, 10 + i)),
  ];
  const adapter = { ...createDemoAdapter(), listEvents: async () => wire.map(mapEvent) };
  renderWithApp(<ConditionFeed />, adapter);
  const feed = await screen.findByRole("list", { name: "Recent container condition changes" });
  const rows = [...feed.querySelectorAll("li")].map((li) => li.textContent);
  expect(rows.length).toBe(5);
  expect(rows[0]).toMatch(/^web exited unexpectedly \(exit 137\)/);
  expect(rows[1]).toMatch(/^db is in a restart loop \(4 restarts in 10 min\)/);
  expect(rows.some((row) => row?.includes("e7"))).toBe(false);
});
