// Source: docklet-next/issues/280
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original proof that every image overview layout keeps the findings chip.

import { screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { securityFixture } from "@/data/http/vulnerabilities.test-support";
import { renderWithApp } from "@/test/render";

import { imageColumns } from "./image-columns";
import { ImageRow } from "./ImageRow";

afterEach(() => { setAppLocale("de"); localStorage.clear(); });

for (const view of ["condensed", "table"] as const) {
  it(`keeps the findings column in the ${view} layout`, () => {
    expect(imageColumns((c) => c, false, view).some((c) => c.id === "security")).toBe(true);
    expect(imageColumns((c) => c, true, view).some((c) => c.id === "security")).toBe(true);
  });
}

// Cards reuse the full column set; condensed has its own and is rendered here.
for (const view of ["condensed", "cards"] as const) {
  it(`renders the findings link in the ${view} layout`, async () => {
    const f = securityFixture(), images = await f.adapter.listImages(), hosts = await f.adapter.listHosts();
    const image = images.find((i) => i.id === "first~sha256:user")!;
    const columns = imageColumns((c) => c, false, view === "cards" ? "table" : view);
    const row = <ImageRow image={image} host={hosts.find((h) => h.id === image.hostId)} view={view} columns={columns} onOpen={vi.fn()} onPull={vi.fn()} onRemove={vi.fn()} />;
    renderWithApp(view === "cards" ? <ul>{row}</ul> : <table><tbody>{row}</tbody></table>, f.adapter);
    const link = await screen.findByText("Gescannte Plattform: linux/amd64");
    expect(link !== null).toBe(true);
    expect(screen.getAllByRole("link").some((l) => l.getAttribute("href") === "/security?tab=images")).toBe(true);
  });
}
