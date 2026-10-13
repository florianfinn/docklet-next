import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useSectionDraft } from "./use-section-draft";

type V = { hubName: string; other: string };
const A: V = { hubName: "A", other: "x" };

function setup(initial: V | V[] = A) {
  return renderHook(({ saved }: { saved: unknown }) => useSectionDraft(saved as V), { initialProps: { saved: initial } });
}

describe("section draft commit", () => {
  it("keeps input typed while the save was running", () => {
    const h = setup();
    act(() => h.result.current.patch({ hubName: "B" }));
    const submitted = h.result.current.draft!;
    act(() => h.result.current.patch({ hubName: "C" }));
    h.rerender({ saved: submitted });
    act(() => h.result.current.commit(submitted, submitted));
    expect(h.result.current.draft).toEqual({ hubName: "C", other: "x" });
    expect(h.result.current.dirty).toBe(true);
  });

  it("takes server normalisation of untouched fields", () => {
    const h = setup();
    act(() => h.result.current.patch({ hubName: "B" }));
    const submitted = h.result.current.draft!;
    act(() => h.result.current.patch({ hubName: "C" }));
    h.rerender({ saved: { hubName: "B", other: "X!" } });
    act(() => h.result.current.commit({ hubName: "B", other: "X!" }, submitted));
    expect(h.result.current.draft).toEqual({ hubName: "C", other: "X!" });
  });

  it("becomes clean without intermediate input", () => {
    const h = setup();
    act(() => h.result.current.patch({ hubName: "B" }));
    const submitted = h.result.current.draft!;
    h.rerender({ saved: submitted });
    act(() => h.result.current.commit(submitted, submitted));
    expect(h.result.current.dirty).toBe(false);
    expect(h.result.current.draft).toEqual(submitted);
  });

  it("keeps the draft after a failed save and saves it again", () => {
    const h = setup();
    act(() => h.result.current.patch({ hubName: "B" }));
    act(() => h.result.current.patch({ hubName: "C" }));
    expect(h.result.current.dirty).toBe(true);
    const retry = h.result.current.draft!;
    h.rerender({ saved: retry });
    act(() => h.result.current.commit(retry));
    expect(h.result.current.dirty).toBe(false);
    expect(h.result.current.draft).toEqual({ hubName: "C", other: "x" });
  });

  it("commit with a transformed value is clean", () => {
    const h = setup();
    act(() => h.result.current.patch({ hubName: "B" }));
    const submitted = h.result.current.draft!;
    h.rerender({ saved: { hubName: "B", other: "y" } });
    act(() => h.result.current.commit({ hubName: "B", other: "y" }, submitted));
    expect(h.result.current.dirty).toBe(false);
    expect(h.result.current.draft).toEqual({ hubName: "B", other: "y" });
  });

  it("keeps a whole array draft that changed during the save", () => {
    const h = setup([{ hubName: "A", other: "x" }]);
    const submitted = [{ hubName: "A", other: "x" }, { hubName: "N", other: "n" }];
    act(() => h.result.current.setDraft(submitted as never));
    const later = [...submitted, { hubName: "M", other: "m" }];
    act(() => h.result.current.setDraft(later as never));
    h.rerender({ saved: submitted as never });
    act(() => h.result.current.commit(submitted as never, submitted as never));
    expect(h.result.current.draft).toEqual(later);
    expect(h.result.current.dirty).toBe(true);
  });
});
