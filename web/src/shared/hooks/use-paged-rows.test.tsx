// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original page clamp, target restoration and identity guards.

import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { usePagedRows } from "./use-paged-rows";

const rows = Array.from({ length: 10_000 }, (_, i) => ({ key: `row-${i}` }));

describe("usePagedRows", () => {
  it("bounds every page and clamps after deletion without losing entry identity", () => {
    const { result, rerender } = renderHook(({ data }) => usePagedRows(data, 200), { initialProps: { data: rows } });
    act(() => result.current.setPage(50));
    expect([result.current.page, result.current.from, result.current.to, result.current.visible.length]).toEqual([50, 9801, 10000, 200]);
    rerender({ data: rows.slice(0, 201) });
    expect([result.current.page, result.current.visible.map((r) => r.key)]).toEqual([2, ["row-200"]]);
    rerender({ data: [] });
    expect([result.current.page, result.current.pageCount, result.current.from, result.current.to]).toEqual([1, 1, 0, 0]);
  });

  it("reanchors included, excluded, late and restored targets synchronously", () => {
    const { result, rerender } = renderHook(({ data, resetKey }) => usePagedRows(data, 100, { targetKey: "row-4999", resetKey }), { initialProps: { data: rows, resetKey: "a" } });
    expect([result.current.page, result.current.targetHidden]).toEqual([50, false]);
    act(() => result.current.setPage(2));
    rerender({ data: rows.toReversed(), resetKey: "b" });
    expect(result.current.page).toBe(51);
    rerender({ data: rows.slice(0, 250), resetKey: "c" });
    expect([result.current.page, result.current.targetHidden]).toEqual([1, true]);
    rerender({ data: rows, resetKey: "c" });
    expect([result.current.page, result.current.targetHidden]).toEqual([50, false]);
  });
});
