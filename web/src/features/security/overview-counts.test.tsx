// Source: docklet-next/issues/609
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #609 #610 security overview count and fixable tile regressions.

import { act, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { securityFixture } from "@/data/http/vulnerabilities.test-support";
import type { SecurityOverview } from "@/domain/model";
import { formatRelative } from "@/domain/format";
import { renderWithApp } from "@/test/render";

import { enSecurity } from "./messages/en";
import { deSecurity } from "./messages/de";
import { OverviewTab } from "./SecurityTabs";

afterEach(() => setAppLocale("de"));

// Exercise the real adapter: the running image has two consumers, while unused, stopped,
// clean scanned and unscanned images each occupy one inventory row.
async function overviewFixture() {
  const f = securityFixture();
  f.inventory.state.hostIds = ["first"];
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => {
    const response = await original(input, init);
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (!path.endsWith("/images") && !path.endsWith("/containers") && !path.endsWith("/summaries") && !path.endsWith("/vulnerabilities/all")) return response;
    const body = await response.json();
    if (path.endsWith("/images")) {
      body.data.push(...["stopped", "unscanned"].map((id) => ({ ...body.data[1], id, repoTags: [`${id}:1`] })));
      body.pagination.totalItems = body.data.length;
    } else if (path.endsWith("/containers")) {
      body.data.push({ ...body.data[1], id: "stopped-consumer", names: ["/stopped-consumer"], imageId: "stopped" });
      body.pagination.totalItems = body.data.length;
    } else if (path.endsWith("/summaries")) {
      const scanned = body.data.summaries["sha256:user"];
      for (const id of ["unused", "dangling", "stopped"]) body.data.summaries[id] = { ...scanned, imageId: id };
    } else {
      body.data.push(
        { ...body.data[0], imageId: "unused", vulnerabilityId: "CVE-2026-1001" },
        { ...body.data[0], imageId: "stopped", vulnerabilityId: "CVE-2026-1002", severity: "LOW", fixedVersion: "" },
      );
      body.pagination.totalItems = body.data.length;
    }
    return new Response(JSON.stringify(body), { status: response.status, headers: response.headers });
  });
  const data = await f.adapter.getSecurity();
  return { ...f, data };
}

function overview(data: SecurityOverview) {
  return <OverviewTab {...data} history={[]} stacks={[]} hosts={new Map()} animate={false} onFocus={vi.fn()} />;
}

function metric(label: string) {
  return screen.getByText(label, { selector: "dt span" }).closest('[role="group"]') as HTMLElement;
}

describe.each(["en-US", "de"] as const)("overview counts in %s", (locale) => {
  const t = locale === "de" ? deSecurity : enSecurity;
  it("counts affected images once per usage and retains finding totals and scanned coverage", async () => {
    const f = await overviewFixture();
    expect(f.data.images.map((image) => [image.usage, image.counts.critical + image.counts.high + image.counts.low + (image.counts.unknown ?? 0)])).toEqual([
      ["running", 6], ["unused", 1], ["unused", 0], ["stopped", 1], ["unused", 0],
    ]);
    setAppLocale(locale);
    renderWithApp(overview(f.data), f.adapter);
    const usage = screen.getByRole("heading", { name: t.share.exposure }).parentElement!;
    for (const label of [t.share.running, t.share.stopped, t.share.unused]) {
      expect(within(usage).getByText(label).querySelector("b")?.textContent).toBe("1");
    }
    expect(screen.getByRole("heading", { name: t.share.severity }) !== null).toBe(true);
    expect(metric(t.strip.critical).querySelector("dd > span")?.textContent).toBe("2");
    expect(metric(t.strip.high).querySelector("dd > span")?.textContent).toBe("1");
    const scanned = metric(t.strip.images);
    expect(scanned.querySelector("dd > span")?.textContent).toBe(locale === "de" ? "4 von 5 Images" : "4 of 5 images");
    expect(scanned.textContent?.includes(formatRelative(f.data.images[0]!.scannedAt))).toBe(true);
  });

  it("counts only readable images in the scanned total and uses the singular for one image", async () => {
    const f = await overviewFixture();
    setAppLocale(locale);
    const stale = { ...f.data.images[0]!, id: "0~sha256:stale", unavailable: true };
    const view = renderWithApp(overview({ ...f.data, images: [...f.data.images, stale] }), f.adapter);
    expect(metric(t.strip.images).querySelector("dd > span")?.textContent).toBe(locale === "de" ? "4 von 5 Images" : "4 of 5 images");
    view.unmount();
    const one = f.data.images.filter((image) => image.scannedAt).slice(0, 1);
    renderWithApp(overview({ ...f.data, images: one }), f.adapter);
    expect(metric(t.strip.images).querySelector("dd > span")?.textContent).toBe(locale === "de" ? "1 von 1 Image" : "1 of 1 image");
  });

  it("explains fixable-only reporting and links to the existing scan settings section", async () => {
    const f = await overviewFixture();
    setAppLocale(locale);
    renderWithApp(overview(f.data), f.adapter);
    await screen.findByText(t.strip.onlyFixable);
    const tile = metric(t.strip.reported);
    expect(tile.querySelector("dd > span")?.textContent).toBe("8");
    expect(within(tile).getByRole("link", { name: t.strip.scanSettings }).getAttribute("href")).toBe("/settings/security");
    expect(tile.textContent?.includes(t.strip.fixableSub.replace("{total}", "8"))).toBe(false);
  });

  it("shows fixable findings out of every active finding when unfixable findings are reported", async () => {
    const f = await overviewFixture();
    f.state.settings.trivyIgnoreUnfixed = "false";
    setAppLocale(locale);
    renderWithApp(overview(f.data), f.adapter);
    await screen.findByText(t.strip.fixableSub.replace("{total}", "8"));
    expect(metric(t.strip.fixable).querySelector("dd > span")?.textContent).toBe("7");
    expect(within(metric(t.strip.fixable)).queryByRole("link") === null).toBe(true);
  });
});

it.each(["denied", "missing", "error", "pending"] as const)("keeps the fixable tile neutral when scan settings are %s", async (state) => {
  const f = await overviewFixture();
  setAppLocale("en-US");
  const original = f.adapter.getScanSettings!;
  let resolve!: () => void;
  const gate = new Promise<void>((done) => { resolve = done; });
  const get = vi.fn(async () => {
    if (state === "error") throw new Error("Settings unavailable");
    await gate;
    return original();
  });
  f.adapter.getScanSettings = state === "missing" ? undefined : get;
  const capability = f.adapter.getCapability;
  const read = vi.spyOn(f.adapter, "getCapability").mockImplementation(async (operation, ref) => {
    const result = await capability(operation, ref);
    return operation === "getScanSettings" && state === "denied" ? { ...result, available: false } : result;
  });
  renderWithApp(overview(f.data), f.adapter);
  await waitFor(() => expect(read.mock.results.some((result) => result.type === "return")).toBe(true));
  if (state === "error" || state === "pending") {
    await waitFor(() => expect(get.mock.calls.length).toBe(1));
    if (state === "error") await act(async () => { await Promise.allSettled(get.mock.results.map((result) => result.value)); });
  }
  else {
    await act(async () => { await Promise.all(read.mock.results.map((result) => result.value)); });
    expect(get.mock.calls.length).toBe(0);
  }
  const tile = metric(enSecurity.strip.fixable);
  expect(tile.querySelector("dd > span")?.textContent).toBe("7");
  expect(tile.querySelectorAll("dd > span").length).toBe(1);
  expect(within(tile).queryByRole("link") === null).toBe(true);
  if (state === "pending") {
    await act(async () => { resolve(); });
    expect((await screen.findByText(enSecurity.strip.onlyFixable)) !== null).toBe(true);
  }
});

it("renders the demo overview without scan settings", async () => {
  const adapter = createDemoAdapter();
  expect(adapter.getScanSettings === undefined).toBe(true);
  setAppLocale("en-US");
  renderWithApp(overview(await adapter.getSecurity()), adapter);
  expect(metric(enSecurity.strip.fixable).querySelectorAll("dd > span").length).toBe(1);
  expect(within(metric(enSecurity.strip.fixable)).queryByRole("link") === null).toBe(true);
});

it("keeps German security catalogue keys and placeholders complete", () => {
  const leaves = (value: object, prefix = ""): [string, string[]][] => Object.entries(value).flatMap(([key, text]) => typeof text === "string"
    ? [[`${prefix}${key}`, [...text.matchAll(/\{(\w+)(?:[,}])/g)].map((match) => match[1]!).sort()]]
    : leaves(text, `${prefix}${key}.`));
  expect(leaves(deSecurity)).toEqual(leaves(enSecurity));
});
