// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231; M21 reads the pattern-tag checker fixture; #634 build dates.

import { describe, expect, it } from "vitest";
import { mapStack, type ProjectRow } from "./inventory-mapping";
import { mapUpdateInfo } from "./update-mapping";
import { hasUpdate, summarizeUpdates } from "@/domain/updates";
import { checkTimes } from "@/shared/components/updates/update-times";
import type { components } from "@/api/generated/schema";
import patternTags from "./testdata/pattern-tag-updates.json";

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
  it("labels same-tag rebuilds by build date and never by an echoed tag or digest", () => {
    const latestTag = { repository: "example", tag: "latest", deployed: "2026-11-10T07:00:00Z" };
    const digest = (c: string) => `sha256:${c.repeat(64)}`;
    const unlabelled = { ...observation("rebuild"), imageRef: "example:latest", currentVersion: "latest", latestVersion: "latest", currentDigest: digest("a"), latestDigest: digest("b"), updateType: "digest", platform: "linux/amd64" };
    const dated = mapUpdateInfo({ ...unlabelled, currentCreated: "2026-11-12T08:00:00Z", latestCreated: "2026-11-24T06:00:00Z" }, latestTag, "example:latest");
    expect(dated).toMatchObject({ state: "rebuild", current: { published: "2026-11-12T08:00:00Z" }, latest: { tag: "latest", published: "2026-11-24T06:00:00Z" } });
    expect(dated.state === "rebuild" && dated.latest.version).toBe(undefined);
    // Without build dates the running image keeps its deploy date; the candidate inherits nothing.
    const undated = mapUpdateInfo(unlabelled, latestTag, "example:latest");
    expect(undated.current).toEqual({ repository: "example", tag: "latest", version: "latest", deployed: "2026-11-10T07:00:00Z" });
    expect(undated.state === "rebuild" && undated.latest).toEqual({ repository: "example", tag: "latest" });
    expect(mapUpdateInfo({ ...unlabelled, latestVersion: "1.2.3" }, latestTag, "example:latest")).toMatchObject({ latest: { version: "1.2.3" } });
    expect(() => mapUpdateInfo({ ...unlabelled, latestCreated: "yesterday" }, latestTag, "example:latest")).toThrow();
  });
  it("ignores a rebuild version that only repeats the running one", () => {
    const patterned = { repository: "example", tag: "release-1.0.0" };
    const echoed = { ...observation("rebuild"), imageRef: "example:release-1.0.0", currentVersion: "1.0.0", latestVersion: "1.0.0", latestCreated: "2026-11-24T06:00:00Z", updateType: "digest", platform: "linux/amd64" };
    const tagPolicy = mapUpdateInfo(echoed, patterned, "example:release-1.0.0");
    expect(tagPolicy.state === "rebuild" && tagPolicy.latest).toEqual({ ...patterned, published: "2026-11-24T06:00:00Z" });
    const droppedLabel = mapUpdateInfo({ ...echoed, imageRef: "example:stable", currentVersion: "1.2.3", latestVersion: "1.2.3" }, { repository: "example", tag: "stable" }, "example:stable");
    expect(droppedLabel.state === "rebuild" && droppedLabel.latest.version).toBe(undefined);
    expect(mapUpdateInfo({ ...echoed, latestVersion: "1.0.1" }, patterned, "example:release-1.0.0")).toMatchObject({ latest: { version: "1.0.1" } });
  });
  it("does not call a partially checked stack current", () => {
    expect(summarizeUpdates([{ name: "web", update: mapUpdateInfo(observation("current"), image, "example:1.0.0") }, { name: "db", update: { state: "unchecked", current: image } }]).visual).toBe("unchecked");
  });
});

it("keeps per-service policy observations separate for a shared image reference", () => {
  const services: NonNullable<ProjectRow["runtimeServices"]> = [{ name: "web", image: "example:1.0.0", status: "running" }, { name: "db", image: "example:1.0.0", status: "running" }];
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


// The hover names missing images from producer counts; only when they are the sole gap do they replace "not checked".
it("names missing images from producer counts without hiding other gaps", () => {
  const missing = (ref: string) => ({ imageRef: ref, updateInfo: { ...observation("unknown"), imageRef: ref, updateType: "not_pulled" } });
  const runtimeServices: NonNullable<ProjectRow["runtimeServices"]> = [{ name: "web", image: "example:1.0.0", status: "running" }, { name: "a1", image: "example/a:1", status: "running" }, { name: "a2", image: "example/a:1", status: "running" }];
  const updateInfo: ProjectRow["updateInfo"] = { status: "not_pulled", hasUpdate: false, imageCount: 2, checkedImageCount: 2, imagesWithUpdates: 0, imagesNotPulled: 1, notPulledImageRefs: ["example/a:1"], errorCount: 0, serviceUpdates: { web: { imageRef: "example:1.0.0", updateInfo: observation("current") }, a1: missing("example/a:1"), a2: missing("example/a:1") } };
  const row: ProjectRow = { id: "project", name: "Example", tags: [], status: "running", isArchived: false, serviceCount: 3, runningCount: 3, path: "/srv/example", createdAt: "2026-10-26T00:00:00Z", updatedAt: "2026-10-26T00:00:00Z", runtimeServices, updateInfo };
  const times = (info: ProjectRow["updateInfo"], services = runtimeServices) => { const m = mapStack({ ...row, runtimeServices: services, updateInfo: info }, "host", [], { stale: false }); return checkTimes(m.services.map((s) => s.update), m.updateGaps); };
  // Shared missing image a is the only gap: checked neighbours stay checked and a counts once.
  expect(times(updateInfo)).toMatchObject({ unchecked: 0, notPulled: 1 });
  // An unattributed missing image b counts too.
  expect(times({ ...updateInfo, imagesNotPulled: 2, notPulledImageRefs: ["example/a:1", "example/b:1"] })).toMatchObject({ unchecked: 0, notPulled: 2 });
  // Unrecorded or failed images keep the unchecked count beside the missing one.
  expect(times({ ...updateInfo, imageCount: 3 })).toMatchObject({ unchecked: 3, notPulled: 1 });
  expect(times({ ...updateInfo, errorCount: 1 })).toMatchObject({ unchecked: 3, notPulled: 1 });
  // A service without its own observation stays unchecked although its reference counts as checked.
  expect(times(updateInfo, [...runtimeServices, { name: "web2", image: "example:2.0.0", status: "running" }])).toMatchObject({ unchecked: 1, notPulled: 1 });
});

it("version observations accept release build metadata without changing update level", () => {
  expect(mapUpdateInfo({ ...observation("version"), currentVersion: "v1.0.0+current.7", latestVersion: "1.1.0+candidate.9" }, image, "example:1.0.0")).toMatchObject({ state: "version", level: "minor", latest: { version: "1.1.0+candidate.9" } });
  for (const currentVersion of ["01.0.0+build", "1.0.0-rc.1+build", "9007199254740992.0.0+build"]) expect(mapUpdateInfo({ ...observation("version"), currentVersion }, image, "example:1.0.0").state).toBe("unknown");
});

// The backend checker test produces this fixture from real tag-policy selections.
type PatternTagCase = { name: string; currentTag: string; update: Pick<components["schemas"]["ImageUpdateInfo"], "state" | "hasUpdate" | "updateType" | "currentVersion" | "latestVersion"> };
it.each(patternTags as PatternTagCase[])("shows the $name tag-policy selection as a version update", ({ currentTag, update }) => {
  const reference = `registry.example.com/app:${currentTag}`;
  const mapped = mapUpdateInfo({ ...observation("version"), ...update, imageRef: reference }, { repository: "registry.example.com/app", tag: currentTag }, reference);
  expect(mapped).toMatchObject({ state: "version", level: "minor", current: { version: "1.0.0" }, latest: { version: "1.1.0" } });
  expect(hasUpdate(mapped)).toBe(true);
  expect(summarizeUpdates([{ name: "app", update: mapped }]).pending).toBe(1);
});
