import { describe, expect, it } from "vitest";

import componentsJson from "../../../components.json?raw";

// The prototype ships only own code on free shadcn/ui primitives; no licensed derivative, no runtime set switch.
const sources = import.meta.glob<string>(["/src/**/*.{ts,tsx,css}", "!/src/**/free-only.test.ts"], { query: "?raw", import: "default", eager: true });

describe("free-only building blocks", () => {
  it("contains no Premium derivative, registry reference or set switch", () => {
    const offenders = Object.entries(sources)
      .filter(([, code]) => /shadcnblocks|\/premium\/|component-set\/(store|switchable)|data-component-set|toggleComponentSet/i.test(code))
      .map(([path]) => path);
    expect(offenders).toEqual([]);
    expect(componentsJson.includes("registries") || componentsJson.includes("shadcnblocks")).toBe(false);
  });

  it("routes every shared building block statically to its free implementation", () => {
    expect(sources["/src/features/activity/ActivityCenter.tsx"]?.includes('from "./RuntimeProgressList"')).toBe(true);
    const sites: [string, string][] = [
      ["/src/features/activity/RuntimeProgressList.tsx", "progress-list"],
      ["/src/features/containers/ContainersPage.tsx", "bulk-action-bar"],
      ["/src/features/containers/ContainersPage.tsx", "stat-strip"],
      ["/src/features/containers/OriginBadge.tsx", "info-hover-card"],
      ["/src/features/dashboard/HostPulse.tsx", "sparkline"],
      ["/src/features/security/SecurityTabs.tsx", "stat-strip"],
      ["/src/features/stacks/overview/StacksPage.tsx", "bulk-action-bar"],
      ["/src/features/stacks/overview/StacksPage.tsx", "stat-strip"],
    ];
    for (const [site, block] of sites) {
      expect(sources[site]?.includes(`from "@/shared/component-set/${block}"`), `${site} → ${block}`).toBe(true);
      expect(sources[`/src/shared/component-set/${block}.ts`]?.includes('from "@/shared/free/'), block).toBe(true);
    }
    expect(sources["/src/app/shell/ShellSidebar.tsx"]?.includes('from "./free/FreeSidebar"')).toBe(true);
  });
});
