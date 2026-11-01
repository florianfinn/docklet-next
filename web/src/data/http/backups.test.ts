// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original backup contract and asynchronous ownership regressions.

import { describe, expect, it } from "vitest";
import { createTranslator } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { BackupError, backupCodeKey, backupErrorMessage } from "@/data/backup-error";
import { mapActivityDetail } from "./map-dto";
import { detail } from "./activity-reader.test-support";
import { backupFixture, ok, policy, overview } from "./backups.test-support";
import { backupCron, backupSchedule } from "./backup-mapping";

function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }

describe("live backup contract", () => {
  it("maps safety/partial/unavailable and preserves six-field custom cron", async () => {
    const f = backupFixture(); const ov = await f.api.getBackupOverview("first");
    expect(ov.volumes[0]?.lastBackup).toMatchObject({ status: "partial", trigger: "safety", errorCode: "volume.backup.restart-failed", sizeBytes: 42 });
    f.state.overview.volumes![0]!.lastBackup!.size = -1;
    expect((await f.api.getBackupOverview("first")).volumes[0]?.lastBackup?.sizeBytes).toBeUndefined();
    const custom = "0 0 2 1 * *";
    expect(backupSchedule(custom)).toEqual({ mode: "custom", cron: custom }); expect(backupCron(backupSchedule(custom))).toBe(custom);
    f.state.overview.volumes![0]!.lastBackup!.status = "unavailable";
    expect((await f.api.getBackupOverview("first")).volumes[0]?.lastBackup?.status).toBe("unavailable");
  });
  it("sends host authority and set version, legacy local flag and count retention", async () => {
    const f = backupFixture(); await f.api.setBackupPolicy("first~data", { enabled: true, targetId: "local", retentionCount: 0, stopContainers: true, schedule: { mode: "weekly", day: "sun", time: "04:15" } });
    expect(f.state.requests.find((r) => r.method === "PUT")?.body).toEqual({ authority: "authority", setVersion: 7, policies: [{ enabled: true, localEnabled: true, retentionCount: 0, stopContainers: true, targetId: "local", schedule: "0 15 4 * * 0" }] });
  });
  it.each(["en-US", "de"] as const)("translates all problem codes and upload reasons in %s without server text", async (locale) => {
    const f = backupFixture(); f.state.fail = "/backups"; f.state.code = "backup.target.stale-set";
    const error = await f.api.runBackup("first~data").catch((e: unknown) => e);
    const t = createTranslator({ locale, messages: catalogs[locale] });
    expect(backupErrorMessage(error, t)).toBe(t("volumes.backupCodes.backup_target_stale-set"));
    for (const key of Object.keys(catalogs[locale].volumes.backupCodes)) expect(t.has(`volumes.backupCodes.${key}` as never)).toBe(true);
    const upload = new BackupError({ code: "volume.backup.upload-invalid", reason: "expanded-too-large" });
    expect(backupErrorMessage(upload, t)).toContain(t("volumes.uploadReasons.expanded-too-large"));
    expect(backupErrorMessage(error, t)).not.toContain("PRIVATE"); expect(backupCodeKey("arbitrary.server.error")).toBeUndefined();
  });
  it.each([
    ["read", 502, "agent.invalid-token", "agent.invalid-token", "credentialRejected"],
    ["read", 409, "agent.update-required", "agent.update-required", "updateRequired"],
    ["read", 502, "agent.unavailable", "agent.unavailable", "unreachable"],
    ["write", 502, "agent.invalid-token", "agent.invalid-token", "credentialRejected"],
    ["write", 502, "agent.unavailable", "volume.outcome-unknown", undefined],
  ] as const)("keeps the %s access refusal %i %s as its own translated code", async (kind, status, code, kept, text) => {
    const f = backupFixture();
    f.intercept(async () => Response.json({ success: false, code }, { status }));
    const error = await (kind === "read" ? f.api.getBackupRepository("first") : f.api.checkBackupRepository("first")).catch((e: unknown) => e);
    expect(error).toMatchObject({ outcome: { kind: "error", code: kept } });
    const t = createTranslator({ locale: "de", messages: catalogs.de });
    expect(backupErrorMessage(error, t)).toBe(t(text ? `common.commandErrors.${text}` : "volumes.errors.outcome-unknown"));
  });
  it("reports busy with its running activity and never repeats the POST", async () => {
    const f = backupFixture(); f.state.fail = "/backups";
    await expect(f.api.runBackup("first~data")).rejects.toMatchObject({ problem: { code: "volume.backup.busy", activityId: "held" } });
    expect(f.accepted).toHaveBeenCalledWith("first", "held"); expect(f.state.requests.filter((r) => r.method === "POST")).toHaveLength(1);
  });
  it("rediscovers a lost admission response from overview without repeating it", async () => {
    const f = backupFixture(); f.state.overview.volumes![0]!.running = { kind: "restore", activityId: "lost", restoreId: "restore" };
    f.intercept(async (url, init) => { if (init?.method === "POST" && url.pathname.endsWith("/restore")) throw new TypeError("lost"); });
    await expect(f.api.restoreBackup("first~data", "snapshot", "replace")).resolves.toBe("lost");
    expect(f.state.requests.filter((r) => r.method === "POST")).toHaveLength(1);
  });
  it("discards held overview and admitted response after host generation changes", async () => {
    const f = backupFixture(), hold = deferred<Response>(), entered = deferred<void>();
    f.intercept(async (url) => { if (url.pathname.endsWith("/overview")) { entered.resolve(); return hold.promise; } });
    const read = f.api.getBackupOverview("first"); const caught = read.catch((error: unknown) => error);
    await entered.promise; f.invalidate(["first"]); hold.resolve(ok(overview));
    await caught;
    await expect(read).rejects.toHaveProperty("silent");
    const next = deferred<Response>(), started = deferred<void>();
    f.intercept(async (_url, init) => { if (init?.method === "POST") { started.resolve(); return next.promise; } });
    const write = f.api.runBackup("first~data"); const failed = write.catch((e: unknown) => e);
    await started.promise; f.rotate(); next.resolve(ok({ activityId: "stale" }, 202));
    await failed; await expect(write).rejects.toHaveProperty("silent"); expect(f.accepted).not.toHaveBeenCalled();
  });
  it("keeps ready hosts while another host is delayed and then unavailable", async () => {
    const f = backupFixture(), hold = deferred<Response>(), entered = deferred<void>(); const batches: string[][] = [];
    f.intercept(async (url) => { if (url.pathname.includes("/second/") && url.pathname.endsWith("/overview")) { entered.resolve(); return hold.promise; } });
    const read = f.api.listBackupOverviews((rows) => batches.push(rows.map((r) => r.hostId)));
    await entered.promise; await new Promise<void>((done) => queueMicrotask(done));
    hold.resolve(Response.json({ code: "agent.unavailable" }, { status: 503 }));
    const rows = await read;
    expect(rows.find((r) => r.hostId === "first")?.volumes).toHaveLength(1); expect(rows.find((r) => r.hostId === "second")?.unavailable).toBe(true); expect(batches.every((batch) => batch.includes("first"))).toBe(true);
  });
  it("maps restore-as-new, files, browse pagination, Edge refusal and staged upload", async () => {
    const f = backupFixture(); await f.api.restoreBackup("first~data", "snapshot", "new", "data-restore-20261028"); await f.api.restoreBackupFiles("first~data", "snapshot", { paths: ["/folder"] }); await f.api.browseBackup("first~data", "snapshot", "/folder", 100);
    expect(f.state.requests.some((r) => r.path.endsWith("/restore") && (r.body as { newVolumeName: string }).newVolumeName === "data-restore-20261028")).toBe(true);
    f.state.fail = "/download"; f.state.code = "volume.backup.download-too-large";
    await expect(f.api.downloadBackup("second~data", "snapshot")).rejects.toMatchObject({ problem: { code: "volume.backup.download-too-large" } });
    f.state.fail = ""; await f.api.uploadRestore("first~data", new File(["abcd"], "archive.tgz"));
    expect(f.state.requests.filter((r) => r.path.includes("/chunks/")).map((r) => r.method)).toEqual(["PUT", "PUT"]);
    expect(f.state.requests.find((r) => r.path.endsWith("/backups/upload"))?.body).toEqual({ mode: "replace", uploadId: "upload" });
  });
  it("maps producer resource-action metadata and typed backup failures without changing generic status", () => {
    const dto = detail("first"); dto.activity.type = "resource_action"; dto.activity.resourceType = "volume"; dto.activity.metadata = { action: "restore_volume_backup" }; dto.activity.status = "failed"; dto.activity.error = "volume.restore.failed-partial";
    const run = mapActivityDetail(dto);
    expect(run.kind).toBe("volume-restore"); expect(run.status).toBe("failed"); expect(run.error?.code).toBe("volumes.backupCodes.volume_restore_failed-partial");
    dto.activity.metadata = { action: "create_volume_backup" };
    expect(mapActivityDetail(dto).kind).toBe("backup");
  });
  it("rediscovers an admission whose accepted response body was lost", async () => {
    const f = backupFixture(); f.state.overview.volumes![0]!.running = { kind: "backup", activityId: "lost-body" };
    f.intercept(async (_url, init) => init?.method === "POST" ? new Response("{", { status: 202 }) : undefined);
    await expect(f.api.runBackup("first~data")).resolves.toBe("lost-body"); expect(f.state.requests.filter((r) => r.method === "POST")).toHaveLength(1);
  });
  it("keeps keys out of target lists and sends local attach to its host", async () => {
    const f = backupFixture(); expect(JSON.stringify(await f.api.listBackupTargets())).not.toContain(f.state.key);
    expect(await f.api.getBackupRecoveryKey("target")).toBe(f.state.key); await f.api.attachLocalBackupRecoveryKey("first", "SYNTHETIC-ATTACH");
    expect(f.state.requests.at(-1)).toMatchObject({ path: "/api/environments/first/volumes/backups/targets/local/recovery-key", method: "PUT", body: { recoveryKey: "SYNTHETIC-ATTACH" } });
    expect(policy.schedule).toBe("0 0 2 * * *");
  });
  it("checks host-local key and test rights on the host and stored-target rights on the hub", async () => {
    const f = backupFixture();
    await f.api.getBackupRecoveryKey("local", "first"); await f.api.getBackupRecoveryKey("target");
    await f.api.testBackupTarget("local", "first").catch(() => undefined); await f.api.testBackupTarget("target", "first").catch(() => undefined);
    const refs = f.capability.mock.calls.filter(([operation]) => operation === "getBackupRecoveryKey" || operation === "testBackupTarget").map(([operation, ref]) => `${operation}:${ref.kind}:${ref.id}`);
    expect(refs).toEqual(["getBackupRecoveryKey:host:first", "getBackupRecoveryKey:hub:0", "testBackupTarget:host:first", "testBackupTarget:hub:0"]);
  });
});
