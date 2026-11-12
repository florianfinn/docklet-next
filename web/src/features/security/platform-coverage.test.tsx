// Source: docklet-next/issues/265
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-container scan platform coverage guards.

import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { useSecurity } from "@/data/queries";
import { mapContainer, type ContainerRow } from "@/data/http/inventory-mapping";
import type { ScannedImage } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { ContainerScanCoverage } from "@/features/containers/ContainerScanCoverage";
import { ImagesTab } from "./SecurityTabs";
import { imageCoverage, normalizePlatform, platformCoverage } from "./platform-coverage";

const observation = { stale: false } as Parameters<typeof mapContainer>[3];
// One multi-platform index on one host: a native, a non-native and an unreported container.
const row = (name: string, imagePlatform: string | undefined) => ({ id: `${name}-id`, names: [`/${name}`], state: "running", status: "Up", image: "registry.example.com/app:1", imageId: "sha256:index", labels: {}, origin: "standalone", originSource: "detected", ...(imagePlatform === undefined ? {} : { imagePlatform }) }) as unknown as ContainerRow;
const containers = [row("native", "linux/amd64"), row("foreign", "linux/arm64"), row("silent", "unknown"), row("old-agent", undefined)].map((r) => mapContainer(r, "example", [], observation));
const scanned: ScannedImage = { id: containers[0]!.imageId, ref: "registry.example.com/app:1", hostId: "example", usage: "running", status: "completed", counts: { critical: 0, high: 0, medium: 0, low: 0, unknown: 0 }, scannedAt: "2026-11-05T03:00:00Z", platform: "linux/amd64" } as ScannedImage;

// Renders once the same security query the hint reads has data, so a missing hint is a settled result.
function Settled() {
  return useSecurity().data ? <span>security-settled</span> : null;
}

describe("platform coverage rule", () => {
  it("compares with the completed scan and never treats an unreported platform as covered", () => {
    expect(platformCoverage("linux/amd64", "linux/amd64")).toBe("covered");
    expect(platformCoverage("linux/arm64/v8", "linux/arm64")).toBe("covered");
    expect(platformCoverage("linux/arm64", "linux/amd64")).toBe("not-covered");
    expect(platformCoverage("unknown", "linux/amd64")).toBe("unknown");
    expect(platformCoverage(undefined, "linux/amd64")).toBe("unknown");
    expect(platformCoverage("linux/arm64", undefined)).toBe(undefined);
  });
  it("implies default variants like the scan side", () => {
    expect(platformCoverage("linux/arm", "linux/arm/v7")).toBe("covered");
    expect(platformCoverage("linux/amd64/v1", "linux/amd64")).toBe("covered");
    expect(platformCoverage("linux/arm/v6", "linux/arm/v7")).toBe("not-covered");
    expect(normalizePlatform("linux/aarch64")).toBe("linux/arm64");
  });
  it("maps the wire platform and an older agent without it to unknown", () => {
    expect(containers.map((c) => c.imagePlatform)).toEqual(["linux/amd64", "linux/arm64", "unknown", "unknown"]);
  });
  it("groups only the same host-qualified image, and nothing before a completed scan", () => {
    expect(imageCoverage(scanned, containers)).toEqual({ "not-covered": ["foreign"], unknown: ["silent", "old-agent"] });
    expect(imageCoverage({ ...scanned, hostId: "other" }, containers)).toEqual({ "not-covered": [], unknown: [] });
    expect(imageCoverage({ ...scanned, scannedAt: "" }, containers)).toEqual({ "not-covered": [], unknown: [] });
  });
});

describe.each(["en-US", "de"] as const)("coverage marker in %s", (locale) => {
  const text = catalogs[locale].security.coverage;
  it("security images view names the non-native and unknown containers separately", () => {
    setAppLocale(locale);
    renderWithApp(<ImagesTab images={[scanned]} stacks={[]} hosts={new Map()} containers={containers} />);
    const body = document.body.textContent ?? "";
    expect(body.includes(text.notCovered.replace("{names}", "foreign"))).toBe(true);
    expect(body.includes(text.unknown.replace("{names}", "silent, old-agent"))).toBe(true);
    expect(body.includes("native,") || body.includes(": native")).toBe(false);
  });
  it.each([
    ["native", undefined],
    ["foreign", "containerNotCovered"],
    ["silent", "containerUnknown"],
  ] as const)("container details for %s", async (name, key) => {
    setAppLocale(locale);
    const adapter = createDemoAdapter();
    const real = await adapter.getSecurity();
    adapter.getSecurity = async () => ({ ...real, images: [scanned] });
    const c = containers.find((item) => item.name === name)!;
    renderWithApp(<><ContainerScanCoverage c={c} /><Settled /></>, adapter);
    await screen.findByText("security-settled");
    if (key === "containerNotCovered") expect((await screen.findByText(text.containerNotCovered.replace("{platform}", "linux/arm64").replace("{scanned}", "linux/amd64"))) !== null).toBe(true);
    else if (key === "containerUnknown") expect((await screen.findByText(text.containerUnknown.replaceAll("''", "'"))) !== null).toBe(true);
    else {
      expect(document.body.textContent?.includes(text.containerNotCovered.slice(0, 12)) || document.body.textContent?.includes(text.containerUnknown.replaceAll("''", "'"))).toBe(false);
    }
  });
});
