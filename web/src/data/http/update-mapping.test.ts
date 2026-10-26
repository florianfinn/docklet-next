// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231.

import { describe, expect, it } from "vitest";
import { mapStack, type ProjectRow } from "./inventory-mapping";
import { mapUpdateInfo } from "./update-mapping";
import { summarizeUpdates } from "@/domain/updates";

import { observation } from "./update-observation.test-support";

const image = { repository: "example", tag: "1.0.0" };
describe("stored update observation", () => {
  it.each(["version", "rebuild", "current", "local", "unknown", "error", "unchecked"] as const)("preserves %s without raw server diagnostics", (state) => {
    const value = mapUpdateInfo({ ...observation(state), code: "registry.authentication-required" }, image, "example:1.0.0", true);
    expect(value).toMatchObject({ state, code: "registry.authentication-required", stale: true });
    expect(JSON.stringify(value)).not.toContain("private raw diagnostic");
  });
  it("refuses mismatched image identity and incomplete version evidence", () => {
    expect(mapUpdateInfo(observation("version"), image, "example:2.0.0").state).toBe("unchecked");
    expect(mapUpdateInfo({ ...observation("version"), latestVersion: "" }, image, "example:1.0.0").state).toBe("unknown");
    expect(() => mapUpdateInfo({ ...observation("current"), checkedAt: "" }, image, "example:1.0.0")).toThrow();
    expect(() => mapUpdateInfo({ ...observation("current"), authRegistry: 42 } as never, image, "example:1.0.0")).toThrow();
  });
  it("does not call a partially checked stack current", () => {
    expect(summarizeUpdates([{ name: "web", update: mapUpdateInfo(observation("current"), image, "example:1.0.0") }, { name: "db", update: { state: "unchecked", current: image } }]).visual).toBe("unchecked");
  });
});

it("keeps per-service policy observations separate for a shared image reference", () => {
  const services = [{ name: "web", image: "example:1.0.0", status: "running" }, { name: "db", image: "example:1.0.0", status: "running" }];
  const row: ProjectRow = { id: "project", name: "Example", tags: [], status: "running", isArchived: false, serviceCount: 2, runningCount: 2, path: "/srv/example", createdAt: "2026-10-26T00:00:00Z", updatedAt: "2026-10-26T00:00:00Z", runtimeServices: services };
  const updateInfo: ProjectRow["updateInfo"] = { checkedImageCount: 2, errorCount: 0, hasUpdate: true, imageCount: 2, imagesNotPulled: 0, imagesWithUpdates: 1, status: "has_update", updateInfoByRef: { "example:1.0.0": observation("version") }, serviceUpdates: { web: { imageRef: "example:1.0.0", updateInfo: observation("current") }, db: { imageRef: "example:1.0.0", updateInfo: observation("rebuild") } } };
  const mapped = mapStack({ ...row, updateInfo }, "host", [], { stale: false });
  expect(mapped.services.map((service) => service.update.state)).toEqual(["current", "rebuild"]);
  updateInfo.serviceUpdates!.db = { imageRef: "example:2.0.0", updateInfo: observation("version") };
  expect(mapStack({ ...row, updateInfo }, "host", [], { stale: false }).services[1]!.update.state).toBe("unchecked");
});

it("incomplete project counts cannot turn known service observations into all-current", () => {
  const row: ProjectRow = { id: "project", name: "Example", tags: [], status: "running", isArchived: false, serviceCount: 1, runningCount: 1, path: "/srv/example", createdAt: "2026-10-26T00:00:00Z", updatedAt: "2026-10-26T00:00:00Z", runtimeServices: [{ name: "web", image: "example:1.0.0", status: "running" }], updateInfo: { status: "unknown", hasUpdate: false, imageCount: 2, checkedImageCount: 1, imagesWithUpdates: 0, imagesNotPulled: 0, errorCount: 0, serviceUpdates: { web: { imageRef: "example:1.0.0", updateInfo: observation("current") } } } };
  const mapped = mapStack(row, "host", [], { stale: false });
  expect(mapped.services[0]!.update.state).toBe("current"); expect(summarizeUpdates(mapped.services).visual).toBe("unchecked");
  expect(() => mapStack({ ...row, updateInfo: { ...row.updateInfo!, imageCount: "two" } as never }, "host", [], { stale: false })).toThrow();
});
