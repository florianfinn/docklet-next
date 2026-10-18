// Source: docklet-next/review/95/acc-review-r1/review-refetch.test.tsx
// Revision: 3f4de335958aada433ee2860554dc8bb356652857fa4f1d029aee4f354c594af
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Run refetch and invalidation in one matrix; assert every rendered state.

import { act } from "@testing-library/react";
import { useQueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { useContainers, useImages, useStacks, useTopology, useVolumes } from "@/data/queries";
import { renderWithApp } from "@/test/render";
import { resourceFixture } from "./resources.test-support";

afterEach(() => vi.useRealTimers());

const hooks = [useStacks, useContainers, useImages, useVolumes, useTopology];
it.each(hooks.flatMap((hook) => ["refetch", "invalidate"].map((mode) => ({ hook, mode }))))("A-B1 keeps loaded rows of a slower host during $mode ($hook.name)", async ({ hook, mode }) => {
  vi.useFakeTimers(); const f = resourceFixture(); const original = f.fetcher.getMockImplementation()!;
  let slow = false;
  f.fetcher.mockImplementation(async (input) => {
    if (slow && String(input).includes("/environments/second/")) await new Promise((resolve) => setTimeout(resolve, 2_000));
    return original(input);
  });
  let refetch: () => void = () => {};
  const seen: string[] = [];
  function Probe() {
    const query = hook(); const client = useQueryClient(); refetch = () => void (mode === "invalidate" ? client.invalidateQueries() : query.refetch());
    const data = query.data; const rows = Array.isArray(data) ? data : data?.networks;
    const text = query.isPending ? "pending" : [...new Set(rows?.map((row) => row.hostId))].sort().join(",");
    seen.push(text);
    return <p>{text}</p>;
  }
  const view = renderWithApp(<Probe />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(500));
  expect(view.container.textContent).toBe("first,second");
  slow = true; seen.length = 0;
  act(() => refetch());
  await act(() => vi.advanceTimersByTimeAsync(500));
  const during = view.container.textContent;
  await act(() => vi.advanceTimersByTimeAsync(3_000));
  expect(seen.every((text) => text === "first,second")).toBe(true);
  expect(view.container.textContent).toBe("first,second");
  expect(during).toBe("first,second");
});
