import { describe, expect, it } from "vitest";

import { filterByHosts, isHostVisible, toggleHost } from "./hosts";
import type { ImageVersion, UpdateInfo } from "./model";
import { levelFromVersions, summarizeUpdates, updateVisual } from "./updates";

const ALL = ["atlas", "boreal", "cedar"];

describe("host filter", () => {
  it("treats an empty selection as all hosts", () => {
    expect(isHostVisible([], "cedar")).toBe(true);
    expect(filterByHosts([{ hostId: "atlas" }, { hostId: "cedar" }], [])).toHaveLength(2);
  });

  it("filters host-bound items by the selection", () => {
    const items = [{ hostId: "atlas", n: 1 }, { hostId: "boreal", n: 2 }, { hostId: "cedar", n: 3 }];
    expect(filterByHosts(items, ["boreal"]).map((i) => i.n)).toEqual([2]);
  });

  it("toggles from 'all' to all-but-one and collapses back to the default", () => {
    const one = toggleHost([], "cedar", ALL);
    expect(one).toEqual(["atlas", "boreal"]);
    expect(toggleHost(one, "cedar", ALL)).toEqual([]);
    expect(toggleHost(["atlas"], "atlas", ALL)).toEqual([]);
  });
});

const img = (tag: string, version?: string): ImageVersion => ({ repository: "redis", tag, version });

describe("update classification", () => {
  it("derives levels only from release versions", () => {
    expect(levelFromVersions("31.0.9", "32.0.1")).toBe("major");
    expect(levelFromVersions("10.10.3", "10.11.1")).toBe("minor");
    expect(levelFromVersions("12.1.1", "12.1.2")).toBe("patch");
  });

  it("keeps a new image state for the same tag as 'rebuild', never as a fix", () => {
    const u: UpdateInfo = { state: "rebuild", current: img("latest", "3.5.0"), latest: img("latest"), checkedAt: "2026-10-10T00:00:00Z", registry: "Docker Hub" };
    expect(updateVisual(u)).toBe("rebuild");
  });

  it("summarizes a stack by its most significant update", () => {
    const s = summarizeUpdates([
      { name: "a", update: { state: "current", current: img("1"), checkedAt: "", registry: "" } },
      { name: "b", update: { state: "rebuild", current: img("7.4"), latest: img("7.4"), checkedAt: "", registry: "" } },
      { name: "c", update: { state: "version", current: img("1.0", "1.0.0"), latest: img("2.0", "2.0.0"), level: "major", checkedAt: "", registry: "" } },
    ]);
    expect(s.pending).toBe(2);
    expect(s.visual).toBe("major");
  });
});
