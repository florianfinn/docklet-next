// Source: docklet-next/issues/584
// Revision: bf8a3a142ddedc4874066ba50c344e13e34dab38
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original host-name proofs for scheduler, update-check and resource runs (M37 #584 acceptance A1).

import { waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";

import { useRunTarget } from "./use-run-target";

afterEach(() => setAppLocale("de"));

const at = "2026-11-23T10:00:00Z";
const atlas = HOSTS.find((h) => h.id === "atlas")!;
function run(type: string, metadata: Record<string, unknown>, resourceType: string, resourceId: string, resourceName?: string) {
  const coded = mapActivityDetail({ activity: { id: "example", environmentId: "atlas", type, status: "success", startedAt: at, createdAt: at, metadata, resourceType, resourceId, ...(resourceName ? { resourceName } : {}) }, messages: [], stepTrace: [], output: [], outputTruncated: false });
  return renderRun(coded, "en-US");
}

it.each([
  ["update check", run("job_run", { jobId: "update-check", environmentId: "atlas" }, "job", "update-check", "Update check"), atlas.name],
  ["auto heal", run("job_run", { jobId: "auto-heal", environmentId: "atlas" }, "job", "auto-heal", "Auto Heal"), atlas.name],
  ["volume backup", run("resource_action", { action: "create_volume_backup" }, "volume", "example-volume", "example-volume"), `example-volume · ${atlas.name}`],
  ["redeploy", run("resource_action", { action: "redeploy" }, "project", "example-stack", "example-stack"), `example-stack · ${atlas.name}`],
])("names the host of a %s run without repeating the operation", async (_name, rendered, expected) => {
  setAppLocale("en-US");
  let target = "";
  function Probe() { target = useRunTarget(rendered); return null; }
  renderWithApp(<Probe />, createDemoAdapter());
  await waitFor(() => expect(target).toBe(expected));
});
