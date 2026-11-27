// Source: docklet-next/issues/680
// Revision: e962f0fe4ab031f8e375d54e9988de3f1c1e8209
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age web consumer and regression coverage.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { response } from "@/data/http/runtime-commands.test-support";
import { policy } from "@/data/http/update-decoders";
import { renderWithApp } from "@/test/render";
import { ConnectedUpdatesSection } from "@/features/settings/sections/ConnectedUpdatesSection";
import { ContainerUpdates } from "@/features/containers/ContainerUpdates";
import type { ContainerInfo } from "@/domain/model";
import { LiveStackUpdates } from "./live-stack-updates";
import { JobUpdateOutcomes } from "./job-update-outcomes";
import { renderRun } from "@/data/message-code";
import { mapActivityDetail } from "@/data/http/map-dto";

const manifest = `sha256:${"a".repeat(64)}`, eligibleAt = "2026-12-01T03:00:00Z";
afterEach(() => setAppLocale("de"));
for (const locale of ["en-US", "de"] as const) {
  for (const kind of ["stack", "container"] as const) {
    it.each([
      { known: true, days: "2", mode: "scheduled" }, { known: false, days: "2", mode: "scheduled" },
      { known: false, days: "0", mode: "scheduled" }, { known: true, days: "0", mode: "scheduled" }, { known: false, days: "2", mode: "check" },
    ] as const)(`${locale} ${kind} HTTP policy age (known=$known, days=$days, mode=$mode)`, async ({ known, days, mode }) => {
      setAppLocale(locale); const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
      f.fetcher.mockImplementation(async (input, init = {}) => String(input).endsWith("/settings") ? response(Object.entries({ effectiveTimeZone: "UTC", updateCheckInterval: "12h", updateNotify: "false", updateMinimumAgeDays: days }).map(([key, value]) => ({ key, value }))) : original(input, init));
      await f.adapter.getCurrentUser();
      f.state.policy.targetKind = kind;
      f.state.policy.mode = mode;
      if (mode === "scheduled") f.state.policy.window = { kind: "daily", time: "04:00" };
      Object.assign(f.state.policy.candidates[0]!, known ? { manifest, eligibleAt } : {});
      // The standalone consumer uses the name-based policy id, separate from its container id.
      const c = { id: "direct~example-id", hostId: "direct", name: "project", origin: "standalone", image: { repository: "example", tag: "1" }, update: { state: "unchecked" } } as ContainerInfo;
      renderWithApp(kind === "stack" ? <LiveStackUpdates stackId="direct~project" /> : <ContainerUpdates c={c} select={() => {}} />, f.adapter);
      await screen.findByRole("combobox");
      const texts = catalogs[locale].common.stackUpdates, formatted = new Intl.DateTimeFormat(locale, { timeZone: "UTC", dateStyle: "medium", timeStyle: "short" }).format(new Date(eligibleAt));
      const waits = texts.waitsUntil.replace("{date}", formatted), shown = () => document.body.textContent ?? "";
      if (days === "0" || mode === "check") {
        // Setting 0 and check-only policies keep the view unchanged: no age text for any candidate shape.
        if (mode === "scheduled") await waitFor(() => expect(f.fetcher.mock.calls.some(([input]) => String(input).endsWith("/settings"))).toBe(true));
        await new Promise((resolve) => setTimeout(resolve, 0));
        expect(shown().includes(waits) || shown().includes(texts.ageUnknown)).toBe(false);
      } else if (known) {
        await waitFor(() => expect(shown().includes(waits)).toBe(true));
        expect(shown().includes(texts.ageUnknown)).toBe(false);
      } else await waitFor(() => expect(shown().includes(texts.ageUnknown)).toBe(true));
    });
  }
  it(`${locale} settings saves and reads minimum age, rejects invalid input and refreshes policies`, async () => {
    setAppLocale(locale); const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
    let age = "0", policyReads = 0, settingsWrites = 0;
    f.fetcher.mockImplementation(async (input, init = {}) => {
      if (String(input).endsWith("/settings")) {
        if (init.method === "PUT") { settingsWrites++; age = JSON.parse(String(init.body)).updateMinimumAgeDays; }
        return response(Object.entries({ effectiveTimeZone: "UTC", updateCheckInterval: "12h", updateNotify: "false", updateMinimumAgeDays: age }).map(([key, value]) => ({ key, value })));
      }
      if (String(input).endsWith("/policies")) policyReads++;
      return original(input, init);
    });
    await f.adapter.getCurrentUser(); renderWithApp(<ConnectedUpdatesSection />, f.adapter);
    const input = await screen.findByRole("spinbutton", { name: catalogs[locale].settings.updates.minimumAge });
    expect((input as HTMLInputElement).value).toBe("0");
    const user = userEvent.setup();
    await user.clear(input); await user.type(input, "91");
    const save = screen.getByRole("button", { name: locale === "de" ? "Speichern" : "Save" });
    expect(input.getAttribute("aria-invalid")).toBe("true");
    await user.click(save); expect(age).toBe("0");
    await user.clear(input); await user.type(input, "1.5"); expect(input.getAttribute("aria-invalid")).toBe("true");
    await user.click(save); expect(age).toBe("0");
    await user.clear(input); await user.type(input, "2");
    await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
    const before = policyReads;
    await user.click(save);
    await waitFor(() => expect(settingsWrites).toBe(1));
    expect(age).toBe("2");
    await waitFor(() => expect(policyReads > before).toBe(true));
    expect((await f.adapter.getUpdateSettings!()).minimumAgeDays).toBe(2);
    expect(document.body.textContent).toContain(catalogs[locale].settings.updates.minimumAgeHelp);
  });
  it(`${locale} job target decodes the minimum age skipped reason`, async () => {
    setAppLocale(locale); const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init = {}) => String(input).includes("/jobs/update-window%3Adirect/runs/example-job") ? response({ id: "example-job", jobId: "update-window:direct", environmentId: "0", outcome: { status: "skipped", targets: [{ id: "example", status: "skipped", code: "update.minimum-age", environmentId: "direct", targetKind: "stack", targetId: "project" }] } }) : original(input, init));
    await f.adapter.getCurrentUser();
    const at = eligibleAt;
    const run = renderRun(mapActivityDetail({ activity: { id: "example", type: "job_run", environmentId: "0", status: "success", startedAt: at, createdAt: at, metadata: { jobId: "update-window:direct", runId: "example-job" } }, messages: [], output: [], stepTrace: [], outputTruncated: false }), locale);
    renderWithApp(<JobUpdateOutcomes run={run} />, f.adapter);
    await waitFor(() => expect(document.querySelector('[data-job-target="example"]') !== null).toBe(true));
    expect(document.body.textContent).toContain(catalogs[locale].images.errors["update_minimum-age"]);
    expect(document.body.textContent).toContain(catalogs[locale].activity.jobStep.skipped);
  });
}
it("rejects malformed manifest and eligible date, preserves missing fields for older agents", () => {
  const f = updateFixture(), raw = structuredClone(f.state.policy);
  expect(policy(raw, "direct", "project").candidates[0]!.manifest).toBeUndefined();
  Object.assign(raw.candidates[0]!, { manifest: "native-key", eligibleAt });
  expect(() => policy(raw, "direct", "project")).toThrow();
  Object.assign(raw.candidates[0]!, { manifest, eligibleAt: "invalid" });
  expect(() => policy(raw, "direct", "project")).toThrow();
});
it.each(["-1", "91", "1.5", "invalid"])("rejects invalid minimum-age setting %s", async (value) => {
  const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init = {}) => String(input).endsWith("/settings") ? response(Object.entries({ effectiveTimeZone: "UTC", updateCheckInterval: "12h", updateNotify: "false", updateMinimumAgeDays: value }).map(([key, value]) => ({ key, value }))) : original(input, init));
  await expect(f.adapter.getUpdateSettings!()).rejects.toHaveProperty("outcome.code", "http.error");
});
