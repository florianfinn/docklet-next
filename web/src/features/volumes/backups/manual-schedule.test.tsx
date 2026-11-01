// Source: docklet-next/issues/291
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original manual-plan schedule preservation regressions.

import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderRoutes } from "@/test/render";
import { backupFixture, policy } from "@/data/http/backups.test-support";
import { mapBackupPolicy } from "@/data/http/backup-mapping";
import { BackupPolicyCard } from "./BackupPolicyCard";
import { withMode } from "./schedule-mode";

describe("manual and disabled volume backup plans (#291)", () => {
  it("keeps a custom cron through manual and save", async () => {
    const custom = "0 30 1 * * 1-5";
    const read = mapBackupPolicy({ ...policy, enabled: true, schedule: custom });
    expect(read.schedule).toEqual({ mode: "custom", cron: custom });
    const manual = { ...read, ...withMode(read.schedule, "manual") };
    expect(manual).toMatchObject({ enabled: false, schedule: { mode: "manual", cron: custom } });
    const f = backupFixture();
    await f.api.setBackupPolicy("first~data", manual);
    expect(f.state.requests.find((r) => r.method === "PUT")?.body).toMatchObject({ policies: [{ enabled: false, schedule: custom }] });
  });

  it("reads a disabled plan as manual with its stored cron and restores it on switch back", () => {
    const read = mapBackupPolicy({ ...policy, enabled: false, schedule: "0 15 4 * * 3" });
    expect(read.schedule).toEqual({ mode: "manual", cron: "0 15 4 * * 3" });
    expect(withMode(read.schedule, "weekly")).toEqual({ enabled: true, schedule: { mode: "weekly", time: "04:15", day: "wed" } });
    expect(withMode(read.schedule, "daily")).toEqual({ enabled: true, schedule: { mode: "daily", time: "04:15" } });
  });

  it("switching a daily plan to manual sends its daily cron, not a fixed default", async () => {
    const manual = withMode({ mode: "daily", time: "05:45" }, "manual");
    const f = backupFixture();
    await f.api.setBackupPolicy("first~data", { ...mapBackupPolicy(policy), ...manual });
    expect(f.state.requests.find((r) => r.method === "PUT")?.body).toMatchObject({ policies: [{ enabled: false, schedule: "0 45 5 * * *" }] });
  });

  it("shows a disabled plan once, as the manual mode, without a separate active switch", async () => {
    const adapter = createDemoAdapter();
    const volume = await adapter.getVolume("vol-atlas-home_ha_config");
    const disabled = { ...volume, backup: { ...volume.backup!, ...mapBackupPolicy({ ...policy, enabled: false, schedule: "0 15 4 * * 3" }), nextRunAt: undefined } };
    renderRoutes([{ path: "/", element: <BackupPolicyCard volume={disabled} targets={[{ id: "local", name: "Local", kind: "local" }]} hostOnline /> }], ["/"], adapter);
    const recurrence = await screen.findByRole("radiogroup", { name: "Wiederholung" });
    expect(within(recurrence).getByRole("radio", { name: "Manuell" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.queryByRole("switch", { name: "Backup aktiv" }) === null).toBe(true);
  });
});
