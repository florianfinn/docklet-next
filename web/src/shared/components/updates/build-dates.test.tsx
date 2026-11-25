// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original same-tag rebuild labels in the stack update panel and chip.

import { screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { formatDate, formatDateTime } from "@/domain/format";
import type { UpdateInfo } from "@/domain/model";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { renderWithApp } from "@/test/render";
import { StackUpdatePanel, UpdateChip } from "./update-panel";

const tag = { repository: "example", tag: "latest" };
const rebuild = (current: object, latest: object): UpdateInfo => ({ state: "rebuild", current: { ...tag, ...current }, latest: { ...tag, ...latest }, checkedAt: "2026-11-25T08:00:00Z", registry: "docker.io" });

for (const locale of ["de", "en-US"] as const) {
  it(`names same-tag rebuilds by build date or short image ID in ${locale}`, () => {
    setAppLocale(locale);
    const copy = catalogs[locale].common.updates;
    const dated = rebuild({ published: "2026-11-12T08:00:00Z" }, { published: "2026-11-24T06:00:00Z" });
    const undated = rebuild({ buildId: "aaaaaaa" }, { buildId: "bbbbbbb" });
    const view = renderWithApp(<><StackUpdatePanel services={[{ name: "web", update: dated }, { name: "db", update: undated }]} /><UpdateChip update={dated} /><UpdateChip update={undated} /></>, updateFixture().adapter);
    const text = document.body.textContent ?? "";
    expect(text.includes(`latest · ${formatDate("2026-11-12T08:00:00Z")}`)).toBe(true);
    expect(text.includes(`latest · ${formatDate("2026-11-24T06:00:00Z")}`)).toBe(true);
    expect(text.includes("latest · bbbbbbb")).toBe(true);
    expect(text.includes("latestlatest")).toBe(false);
    expect(screen.getAllByTitle(copy.noBuildDate).length).toBe(3);
    expect(screen.getAllByTitle(copy.builtAt.replace("{date}", formatDateTime("2026-11-24T06:00:00Z"))).length).toBe(2);
    view.unmount();
  });
}
