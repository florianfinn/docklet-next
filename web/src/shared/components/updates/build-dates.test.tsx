// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original same-tag rebuild labels in the stack update panel and chip; same-day builds stay distinct; deploy and check dates without build dates.

import { screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { formatDate, formatInstant, formatTime } from "@/domain/format";
import type { UpdateInfo } from "@/domain/model";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { renderWithApp } from "@/test/render";
import { StackUpdatePanel, UpdateChip } from "./update-panel";

const tag = { repository: "example", tag: "latest" };
const rebuild = (current: object, latest: object): UpdateInfo => ({ state: "rebuild", current: { ...tag, ...current }, latest: { ...tag, ...latest }, checkedAt: "2026-11-25T08:00:00Z", registry: "docker.io" });

for (const locale of ["de", "en-US"] as const) {
  it(`names same-tag rebuilds by build date, else by deploy and check date, in ${locale}`, () => {
    setAppLocale(locale);
    const copy = catalogs[locale].common.updates;
    const dated = rebuild({ published: "2026-11-12T08:00:00Z" }, { published: "2026-11-24T06:00:00Z" });
    const undated = rebuild({ deployed: "2026-11-10T07:00:00Z" }, {});
    const view = renderWithApp(<><StackUpdatePanel services={[{ name: "web", update: dated }, { name: "db", update: undated }]} /><UpdateChip update={dated} /><UpdateChip update={undated} /></>, updateFixture().adapter);
    const text = document.body.textContent ?? "";
    expect(text.includes(`latest · ${formatDate("2026-11-12T08:00:00Z")}`)).toBe(true);
    expect(text.includes("latestlatest")).toBe(false);
    expect(screen.getAllByText(`latest · ${formatDate("2026-11-24T06:00:00Z")}`).length).toBe(2);
    expect(screen.getAllByTitle(copy.builtAt.replace("{date}", formatInstant("2026-11-24T06:00:00Z"))).length).toBe(2);
    expect(screen.getAllByTitle(copy.deployedAt.replace("{date}", formatInstant("2026-11-10T07:00:00Z"))).length).toBe(1);
    expect(screen.getAllByTitle(copy.checkedAt.replace("{date}", formatInstant("2026-11-25T08:00:00Z"))).length).toBe(2);
    expect(screen.getAllByText(`latest · ${formatDate("2026-11-25T08:00:00Z")}`).length).toBe(2);
    view.unmount();
  });
  it(`leads a rebuild chip with the build date, not a repeated version, in ${locale}`, () => {
    setAppLocale(locale);
    const patterned = { repository: "example", tag: "release-1.0.0" };
    const update: UpdateInfo = { state: "rebuild", current: { ...patterned, version: "1.0.0", published: "2026-11-12T08:00:00Z" }, latest: { ...patterned, version: "1.0.1", published: "2026-11-24T06:00:00Z" }, checkedAt: "2026-11-25T08:00:00Z", registry: "docker.io" };
    const view = renderWithApp(<><StackUpdatePanel services={[{ name: "web", update }]} /><UpdateChip update={update} /></>, updateFixture().adapter);
    const text = document.body.textContent ?? "";
    expect(text.includes(`release-1.0.0 · ${formatDate("2026-11-12T08:00:00Z")}`)).toBe(true);
    expect(text.includes(`1.0.1 · ${formatDate("2026-11-24T06:00:00Z")}`)).toBe(true);
    expect(screen.getAllByText(`1.0.1 · ${formatDate("2026-11-24T06:00:00Z")}`).length).toBe(2);
    view.unmount();
  });
  it(`tells same-day builds apart down to the second in ${locale}`, () => {
    setAppLocale(locale);
    const copy = catalogs[locale].common.updates;
    const [first, second] = ["2026-11-24T06:00:01Z", "2026-11-24T06:00:55Z"];
    const update = rebuild({ published: first }, { published: second });
    const view = renderWithApp(<><StackUpdatePanel services={[{ name: "web", update }]} /><UpdateChip update={update} /></>, updateFixture().adapter);
    const labels = (at: string) => screen.getAllByTitle(copy.builtAt.replace("{date}", formatInstant(at))).map((node) => node.textContent);
    expect(labels(first).length).toBe(1);
    expect(labels(second).length).toBe(2);
    expect(labels(first).every((label) => label?.includes(formatTime(first, true)))).toBe(true);
    expect(labels(second).every((label) => label?.includes(formatTime(second, true)))).toBe(true);
    expect(formatInstant(first) === formatInstant(second)).toBe(false);
    view.unmount();
  });
}
