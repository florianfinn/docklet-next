// Source: docklet-next/issues/330
// Revision: b17a7a70791591d0709db8b3d0b3898fd0d39330
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regression for answered backup writes during a host change (M16 acceptance DV1 E).

import { CancelledError } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { backupFixture } from "./backups.test-support";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

type Fixture = ReturnType<typeof backupFixture>;
const input = { name: "Renamed", kind: "smb" as const, server: "nas.example.com", share: "backups", path: "docklet" };
const writes: [string, string, (f: Fixture) => Promise<unknown>][] = [
  ["update target", "PUT", (f) => f.api.updateBackupTarget!("target", input)],
  ["create target", "POST", (f) => f.api.createBackupTarget!(input)],
  ["delete target", "DELETE", (f) => f.api.deleteBackupTarget!("target")],
  ["run backup", "POST", (f) => f.api.runBackup!("first~data")],
  ["attach local recovery key", "PUT", (f) => f.api.attachLocalBackupRecoveryKey!("first", "SYNTHETIC-RECOVERY-KEY")],
];

// A hub environments event (an agent stopping) invalidates every host while a write is in flight; the answered write
// keeps its outcome, and only an identity change discards it.
describe.each(writes)("%s answered during a host change", (_name, method, run) => {
  async function during(change: (f: Fixture) => void) {
    const f = backupFixture(), started = deferred<void>(), answer = deferred<Response | undefined>();
    f.intercept(async (_url, init) => {
      if ((init?.method ?? "GET") !== method) return undefined;
      started.resolve();
      return answer.promise;
    });
    const write = run(f);
    const settled = write.then((value) => ({ value }), (error: unknown) => ({ error }));
    await started.promise;
    change(f);
    answer.resolve(undefined);
    return { f, outcome: await settled };
  }

  it("keeps the outcome when every host is invalidated", async () => {
    const control = await during(() => undefined);
    const { f, outcome } = await during((fixture) => fixture.invalidate());
    expect("value" in control.outcome).toBe(true);
    expect(outcome).toEqual(control.outcome);
    expect(f.accepted.mock.calls).toEqual(control.f.accepted.mock.calls);
  });

  it("discards the outcome when the identity changes", async () => {
    const { outcome } = await during((fixture) => { fixture.rotate(); });
    expect("error" in outcome && outcome.error instanceof CancelledError).toBe(true);
  });
});

it("keeps a typed write failure when every host is invalidated", async () => {
  const f = backupFixture(), started = deferred<void>(), answer = deferred<Response>();
  f.intercept(async (_url, init) => {
    if (init?.method !== "PUT") return undefined;
    started.resolve();
    return answer.promise;
  });
  const write = f.api.updateBackupTarget!("target", input).catch((error: unknown) => error);
  await started.promise;
  f.invalidate();
  answer.resolve(Response.json({ code: "backup.target.in-use", status: 409 }, { status: 409 }));
  expect(await write).toMatchObject({ problem: { code: "backup.target.in-use" } });
});
