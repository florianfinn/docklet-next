// Source: docklet-next/issues/290
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original Direct streaming download transport regressions.

import { afterEach, describe, expect, it, vi } from "vitest";
import { backupFixture } from "./backups.test-support";

afterEach(() => vi.restoreAllMocks());

describe("volume backup download transport (#290)", () => {
  it("hands Direct and hub-local downloads to the browser without reading the archive", async () => {
    const f = backupFixture();
    const clicks: { href: string; download: string }[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      clicks.push({ href: this.getAttribute("href") ?? "", download: this.download });
    });
    await expect(f.api.downloadBackup("first~data", "snapshot")).resolves.toBeUndefined();
    expect(clicks).toEqual([{ href: "/api/environments/first/volumes/backups/snapshot/download", download: "data-snapshot.tar.gz" }]);
    expect(f.state.requests.some((r) => r.path.endsWith("/download"))).toBe(false);
    expect(document.querySelector("a[download]") === null).toBe(true);
  });

  it("keeps the bounded buffered read and typed refusal on Edge", async () => {
    const f = backupFixture();
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click");
    const blob = await f.api.downloadBackup("second~data", "snapshot");
    expect(blob instanceof Blob).toBe(true);
    expect(await blob!.text()).toBe("archive");
    f.state.fail = "/download"; f.state.code = "volume.backup.download-too-large";
    await expect(f.api.downloadBackup("second~data", "snapshot")).rejects.toMatchObject({ problem: { code: "volume.backup.download-too-large" } });
    expect(click.mock.calls.length).toBe(0);
  });
});
