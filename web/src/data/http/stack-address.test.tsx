// Source: docklet-next/issues/334
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original apps widget address mapping tests. M20 #360: stacks without an address are not parsed.

import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppsWidget } from "@/features/dashboard/widgets/SmallWidgets";
import { renderWithApp } from "@/test/render";
import { mapStack, stackAddress, type ProjectRow } from "./inventory-mapping";

const observed = { stale: false };

function project(id: string, name: string, urls?: string[] | null): ProjectRow {
  return { id, name, status: "running", path: `/srv/${name}`, serviceCount: 1, runningCount: 1, urls } as unknown as ProjectRow;
}

describe("stack address mapping (#334)", () => {
  it("takes the first http(s) address and skips other schemes and malformed entries", () => {
    expect(stackAddress(["mc.example.com:25565", "not a url", "ftp://files.example.com", "https://media.example.com/web"])).toBe("https://media.example.com/web");
    expect(stackAddress(["http://192.0.2.10:8096"])).toBe("http://192.0.2.10:8096/");
    expect(stackAddress(["mc.example.com:25565"])).toBeUndefined();
    expect(stackAddress(null)).toBeUndefined();
    expect(stackAddress(undefined)).toBeUndefined();
  });

  it("maps the address onto the stack and keeps same-name stacks on two hosts apart", () => {
    const a = mapStack(project("p1", "media", ["https://media.example.com"]), "1", [], observed);
    const b = mapStack(project("p1", "media", []), "2", [], observed);
    expect(a.url).toBe("https://media.example.com/");
    expect(b.url).toBeUndefined();
    expect(a.id === b.id).toBe(false);
  });

  it("lists mapped stacks in the apps widget as links that open in a new tab", async () => {
    const stack = mapStack(project("p1", "media", ["https://media.example.com"]), "1", [], observed);
    const bare = mapStack(project("p2", "craft", ["mc.example.com:25565"]), "1", [], observed);
    renderWithApp(<AppsWidget stacks={[stack, bare]} hosts={new Map()} />);
    const link = await screen.findByRole("link", { name: /media/ });
    expect(link.getAttribute("href")).toBe("https://media.example.com/");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(screen.queryByRole("link", { name: /craft/ }) === null).toBe(true);
  });

  afterEach(() => vi.unstubAllGlobals());

  it("parses no address for stacks without one, so dense Dashboards render without thrown errors", async () => {
    const parsed: string[] = [];
    const Native = URL;
    vi.stubGlobal("URL", new Proxy(Native, { construct: (target, args: [string]) => { parsed.push(args[0]); return new target(...args); } }));
    const stack = mapStack(project("p1", "media", ["https://media.example.com"]), "1", [], observed);
    const bare = Array.from({ length: 3 }, (_, i) => mapStack(project(`b${i}`, `bare${i}`, []), "1", [], observed));
    parsed.length = 0;
    renderWithApp(<AppsWidget stacks={[stack, ...bare]} hosts={new Map()} />);
    await screen.findByRole("link", { name: /media/ });
    expect(parsed.includes("")).toBe(false);
  });
});
