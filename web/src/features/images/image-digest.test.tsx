// Source: docklet-next/issues/267
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original proof that live image details show the real digest without a synthetic statement.

import { screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { ImageDetails } from "@/features/images/ImageDetails";
import { renderWithApp } from "@/test/render";
import { securityFixture } from "@/data/http/vulnerabilities.test-support";

afterEach(() => { setAppLocale("de"); localStorage.clear(); });

for (const locale of ["en-US", "de"] as const) {
  it(`shows the real digest without a synthetic statement (${locale})`, async () => {
    setAppLocale(locale);
    const f = securityFixture(), images = await f.adapter.listImages(), hosts = await f.adapter.listHosts();
    const image = images.find((i) => i.id === "first~sha256:user")!;
    renderWithApp(<ImageDetails id={image.id} image={image} host={hosts.find((h) => h.id === image.hostId)} onClose={vi.fn()} onPull={vi.fn()} onRemove={vi.fn()} />, f.adapter);
    expect((await screen.findAllByTitle(image.digest)).length > 0).toBe(true);
    expect(screen.queryByText(/synthetic|synthetisch/i) === null).toBe(true);
  });
}
