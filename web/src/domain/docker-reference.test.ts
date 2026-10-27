// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original Docker identity and producer observation regressions (#231).

import { attentionItems } from "@/features/dashboard/attention";
import { expect, it } from "vitest";
import { normalizeDockerReference, sameImageReference, imageReferenceValue } from "./docker-reference";
import { mapUpdateInfo } from "@/data/http/update-mapping";
import { mapStack } from "@/data/http/inventory-mapping";
import { candidateObservation, candidateProject, candidateUpdateInfo } from "@/data/http/candidate-update.test-support";

const digest = `sha256:${"a".repeat(64)}`;
it.each([
  ["shorthand", "busybox:1.35.0", "docker.io/library/busybox:1.35.0"],
  ["fully qualified", "docker.io/library/busybox:1.35.0", "busybox:1.35.0"],
  ["library", "library/busybox:1.35.0", "docker.io/library/busybox:1.35.0"],
  ["registry with port", "REGISTRY.EXAMPLE.COM:5000/team/app:1.35.0", "registry.example.com:5000/team/app:1.35.0"],
  ["digest", `busybox@${digest}`, `docker.io/library/busybox@${digest}`],
  ["default tag", "busybox", "docker.io/library/busybox:latest"],
  ["legacy Docker registry", "index.docker.io/busybox:1.35.0", "docker.io/library/busybox:1.35.0"],
])("retains producer %s observations across equivalent references", (name, configured, observed) => {
  expect(sameImageReference(configured!, observed!)).toBe(true);
  const info = candidateObservation(observed);
  if (name === "digest") Object.assign(info, { state: "current", hasUpdate: false, latestVersion: info.currentVersion, latestDigest: info.currentDigest, updateType: "" });
  const expected = name === "digest" ? "current" : "version";
  const result = mapUpdateInfo(info, { repository: "busybox", tag: "1.35.0" }, configured!);
  expect(result.state).toBe(expected);
  if (result.state === "version") expect(result).toMatchObject({ level: "minor", latest: { version: "1.38.0" } });
  const updateInfo = candidateUpdateInfo(observed, observed);
  updateInfo.serviceUpdates!.app!.updateInfo = info;
  updateInfo.updateInfoByRef![observed!] = info;
  if (name === "digest") Object.assign(updateInfo, { hasUpdate: false, imagesWithUpdates: 0, status: "up_to_date" });
  const mapped = mapStack({ ...candidateProject, runtimeServices: [{ name: "app", image: configured!, status: "running" }], updateInfo }, "host", [], { stale: false });
  expect(mapped.services[0]!.update.state).toBe(expected);
});
it("normalizes namespaces and default tags without erasing digest or explicit tag", () => {
  expect(normalizeDockerReference("busybox")).toBe("docker.io/library/busybox:latest");
  expect(normalizeDockerReference("team/app")).toBe("docker.io/team/app:latest");
  expect(normalizeDockerReference("registry.example.com:5000/app")).toBe("registry.example.com:5000/app:latest");
  expect(normalizeDockerReference(`busybox@${digest}`)).toBe(`docker.io/library/busybox@${digest}`);
  expect(normalizeDockerReference(`busybox:stable@${digest}`)).toBe(`docker.io/library/busybox:stable@${digest}`);
});
it.each(["busybox:1.36.0", "other/busybox:1.35.0", "registry.example.com/busybox:1.35.0", "busybox:STABLE"])("does not match a different resource %s", (other) => {
  expect(sameImageReference("busybox:1.35.0", other)).toBe(false);
  expect(mapUpdateInfo(candidateObservation(), { repository: "busybox", tag: "1.35.0" }, other).state).toBe("unchecked");
});
it("keeps distinct digests and tag case distinct", () => {
  expect(sameImageReference(`busybox@${digest}`, `busybox@sha256:${"b".repeat(64)}`)).toBe(false);
  expect(sameImageReference("busybox:Stable", "busybox:stable")).toBe(false);
});
it.each(["", "Busybox:latest", "busy box", "busybox:", "busybox@sha256:short", "busybox@@sha256:bad", "/busybox"])("fails closed for invalid reference %s", (ref) => {
  expect(normalizeDockerReference(ref)).toBeUndefined(); expect(sameImageReference(ref, ref)).toBe(false);
});
it("looks up canonical ref-map keys and retains an exact producer key", () => {
  const canonical = candidateObservation(), exact = { ...canonical, state: "unchecked" as const };
  expect(imageReferenceValue({ "docker.io/library/busybox:1.35.0": canonical }, "busybox:1.35.0")).toBe(canonical);
  expect(imageReferenceValue({ "busybox:1.35.0": exact, "docker.io/library/busybox:1.35.0": canonical }, "busybox:1.35.0")).toBe(exact);
  const updateInfo = { ...candidateUpdateInfo(), serviceUpdates: undefined, updateInfoByRef: { "docker.io/library/busybox:1.35.0": canonical } };
  expect(mapStack({ ...candidateProject, updateInfo }, "host", [], { stale: false }).services[0]!.update.state).toBe("version");
});


it("dashboard groups equivalent scanned references without losing the owning image", () => {
  const image = { ref: "busybox:1.35.0", hostId: "host", usage: "running" as const, counts: { critical: 1, high: 0, medium: 0, low: 0 }, scannedAt: "2026-10-26T00:00:00Z" };
  const items = attentionItems({ hosts: [], stacks: [], containers: [], runs: [], images: [image, { ...image, ref: "docker.io/library/busybox:1.35.0" }], metrics: undefined, now: 0 });
  expect(items).toHaveLength(1); expect(items[0]).toMatchObject({ kind: "critical", hostId: "host", values: { count: 2 } });
});
