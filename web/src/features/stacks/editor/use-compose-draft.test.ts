import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ops } from "@/domain/compose/ops";

import { NEW_STACK_DRAFT } from "./new-stack-draft";
import { useComposeDraft } from "./use-compose-draft";

const seed = { compose: NEW_STACK_DRAFT, env: "TZ=Europe/Berlin\n", hostId: "atlas", labels: [] };

describe("synchronized compose draft", () => {
  it("applies Easy edits to the raw text and keeps unknown keys", () => {
    const { result } = renderHook(() => useComposeDraft(seed));
    act(() => {
      result.current.edit(ops.setField("web", "image", "traefik/whoami:v1.12"));
    });
    expect(result.current.compose).toContain("image: traefik/whoami:v1.12");
    expect(result.current.compose).toContain("notiz: Dieses Feld kennt das Formular nicht");
    expect(result.current.dirty).toBe(true);
  });

  it("keeps invalid raw text untouched and rejects form edits until it is fixed", () => {
    const { result } = renderHook(() => useComposeDraft(seed));
    const broken = `${NEW_STACK_DRAFT}\n  oops: [unclosed\n`;
    act(() => result.current.setCompose(broken));
    expect(result.current.parsed.ok).toBe(false);
    let error: unknown;
    act(() => {
      error = result.current.edit(ops.setName("other"));
    });
    expect(error !== null).toBe(true);
    expect(result.current.compose).toBe(broken);
  });

  it("applies consecutive edits in one event to the newest text", () => {
    const { result } = renderHook(() => useComposeDraft(seed));
    act(() => {
      result.current.edit(ops.addService("db", { image: "postgres:17-alpine" }));
      result.current.edit(ops.addService("cache", { image: "redis:7.4-alpine" }));
    });
    const keys = result.current.parsed.ok ? result.current.parsed.model.services.map((s) => s.key) : [];
    expect(keys).toEqual(["web", "db", "cache"]);
  });

  it("reset restores the saved files exactly", () => {
    const { result } = renderHook(() => useComposeDraft(seed));
    act(() => {
      result.current.edit(ops.setName("changed"));
    });
    act(() => result.current.reset());
    expect(result.current.compose).toBe(NEW_STACK_DRAFT);
    expect(result.current.dirty).toBe(false);
  });
});

describe("label dirtiness", () => {
  const withLabels = { ...seed, labels: ["web", "prod"] };

  it("is dirty when only a label changes, and reset restores", () => {
    const { result } = renderHook(() => useComposeDraft(withLabels));
    act(() => result.current.setLabels(["web", "prod", "new"]));
    expect(result.current.dirty).toBe(true);
    expect(result.current.composeDirty).toBe(false);
    act(() => result.current.reset());
    expect(result.current.dirty).toBe(false);
    expect(result.current.labels).toEqual(["web", "prod"]);
  });

  it("ignores order, case and whitespace", () => {
    const { result } = renderHook(() => useComposeDraft(withLabels));
    act(() => result.current.setLabels([" PROD", "Web ", "web"]));
    expect(result.current.dirty).toBe(false);
  });

  it("adopts a normalized server seed silently after markSaved", () => {
    const { result, rerender } = renderHook(({ s }) => useComposeDraft(s), { initialProps: { s: withLabels } });
    act(() => result.current.setLabels(["Web", "PROD"]));
    act(() => result.current.markSaved());
    expect(result.current.dirty).toBe(false);
    rerender({ s: { ...withLabels, labels: ["web", "prod"], env: "TZ=UTC\n" } });
    expect(result.current.env).toBe("TZ=UTC\n");
    expect(result.current.labels).toEqual(["web", "prod"]);
    expect(result.current.dirty).toBe(false);
  });
});
