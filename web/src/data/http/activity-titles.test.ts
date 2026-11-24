// Source: docklet-next/issues/584
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import { readFileSync, readdirSync } from "node:fs";
import { join, relative, resolve as resolvePath } from "node:path";
import { expect, it } from "vitest";
import { renderRun } from "../message-code";
import { mapActivityDetail } from "./map-dto";
import { OPERATION_TITLES, RESOURCE_TITLES, ACTION_TITLES, JOB_PREFIX_TITLES, JOB_TITLES, TYPE_TITLES } from "./activity-titles";

const root = resolvePath(process.cwd(), "..");
function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => entry.isDirectory() ? sources(join(dir, entry.name)) : entry.name.endsWith(".go") && !entry.name.endsWith("_test.go") ? [join(dir, entry.name)] : []);
}
const backend = sources(join(root, "backend")).map((path) => ({ path: relative(root, path), text: readFileSync(path, "utf8") }));
// Resolve aliases such as VulnerabilityScanJobName = vulnerability.ScanJobID.
const definitions = new Map<string, string>();
for (const { text } of backend) for (const m of text.matchAll(/^\s*(?:const\s+)?(\w+)(?:\s+\w+)?\s*=\s*("[^"\n]*"|[\w.]+)/gm)) definitions.set(m[1]!, m[2]!);
function resolve(expression: string, seen = new Set<string>()): string | undefined {
  if (expression.startsWith('"')) return JSON.parse(expression) as string;
  const name = expression.split(".").at(-1)!;
  if (seen.has(name)) return undefined;
  seen.add(name);
  const next = definitions.get(name);
  return next ? resolve(next, seen) : undefined;
}
function functionBodies(text: string): string[] {
  return [...text.matchAll(/func\s+\([^)]*\)\s+Name\(\)\s+string\s*\{/g)].map((match) => {
    const start = match.index + match[0].length;
    let depth = 1, end = start;
    while (depth > 0 && end < text.length) { const c = text[end++]!; if (c === "{") depth++; if (c === "}") depth--; }
    return text.slice(start, end - 1);
  });
}

it("explicitly maps every producer activity type, including candidate scans", () => {
  const text = readFileSync(join(root, "types/activity/activity.go"), "utf8") + readFileSync(join(root, "backend/internal/activity/capabilities.go"), "utf8");
  const types = [...text.matchAll(/Type\w+\s+(?:activitytypes\.)?Type\s*=\s*"([^"]+)"/g)].map((m) => m[1]!);
  expect(types).toContain("vulnerability.candidate-scan");
  expect(types.length).toBeGreaterThan(30);
  expect(types.filter((type) => !Object.hasOwn(TYPE_TITLES, type))).toEqual([]);
});

it("explicitly maps production scheduler Name methods, constants, JobID/JobName fields and dynamic families", () => {
  const jobs = new Set<string>();
  for (const { path, text } of backend) {
    const expressions = [...text.matchAll(/\bJob(?:ID|Name):\s*("[^"]*"|[\w.]+)/g)].map((m) => m[1]!);
    for (const m of text.matchAll(/^\s*(?:const\s+)?(\w*Job(?:Name|ID))\s*=\s*("[^"\n]*"|[\w.]+)/gm)) {
      expressions.push(m[2]!);
      expect(resolve(m[2]!), `${path}: ${m[1]}`).toBeDefined();
    }
    if (path.startsWith("backend/pkg/scheduler/") && path.endsWith("_job.go")) {
      for (const body of functionBodies(text)) for (const m of body.matchAll(/return\s+("[^"]*"|[\w.]+)/g)) {
        expressions.push(m[1]!);
        expect(resolve(m[1]!), `${path}: Name()`).toBeDefined();
      }
    }
    for (const expression of expressions) { const value = resolve(expression); if (value) jobs.add(value); }
    for (const m of text.matchAll(/entityjobs\.New\("([a-z-]+:)"/g)) jobs.add(m[1]!);
    for (const m of text.matchAll(/func\s+\w*Job\w*\([^)]*\)\s+string\s*\{\s*return\s+"([a-z-]+:)"/g)) jobs.add(m[1]!);
  }
  expect([...jobs]).toEqual(expect.arrayContaining(["auto-heal", "history-sample", "notification-delivery-wake", "update-window:", "system-backup:", "volume-backup:"]));
  const mapped = (job: string) => Object.hasOwn(JOB_TITLES, job) || Object.keys(JOB_PREFIX_TITLES).some((prefix) => job.startsWith(prefix));
  expect([...jobs].filter((job) => !mapped(job))).toEqual([]);
});

it("explicitly maps every action discriminator in resource-action producer files", () => {
  const actions = new Set<string>();
  for (const { text } of backend.filter((s) => s.text.includes("TypeResourceAction"))) {
    for (const m of text.matchAll(/"action":\s*("[^"]+"|[\w.]+)/g)) {
      const value = resolve(m[1]!);
      if (value) actions.add(value);
      else {
        // Widget request actions are a validated two-value input, not metadata guessed from prose.
        expect(text).toContain('action != "approve" && action != "decline"');
        for (const match of text.matchAll(/action != "([^"]+)"/g)) actions.add(match[1]!);
      }
    }
  }
  expect([...actions]).toEqual(expect.arrayContaining(["check_environment_connection", "sync_environment", "create_network", "check_volume_backup_repository", "update_tags", "approve", "decline"]));
  expect([...actions].filter((action) => !Object.hasOwn(ACTION_TITLES, action))).toEqual([]);
});

it("names every resource-action producer without an action by a mapped operation or resource kind", () => {
  const unnamed: string[] = [];
  for (const { path, text } of backend.filter((s) => s.text.includes("TypeResourceAction") && !s.text.includes('"action":'))) {
    const operations = [...text.matchAll(/"operation":\s*"([^"]+)"/g)].map((m) => m[1]!);
    const kinds = [...text.matchAll(/ResourceType:\s*(?:new\()?"([^"]+)"/g)].map((m) => m[1]!);
    if (!operations.some((o) => Object.hasOwn(OPERATION_TITLES, o)) && !kinds.some((k) => Object.hasOwn(RESOURCE_TITLES, k))) unnamed.push(path);
  }
  expect(unnamed).toEqual([]);
});

const at = "2026-11-23T10:00:00Z";
function mapped(type: string, metadata: Record<string, unknown> = {}, resourceType = "environment") {
  return mapActivityDetail({ activity: { id: "example", environmentId: "0", type, status: "success", startedAt: at, createdAt: at, metadata, resourceType, resourceId: "0" }, messages: [], stepTrace: [], output: [], outputTruncated: false });
}
it.each([
  ["hub.update", { targetVersion: "1.5.0" }, "Hub update to 1.5.0", "Hub-Update auf 1.5.0"],
  ["hub.update", {}, "Hub update", "Hub-Update"],
  ["vulnerability.candidate-scan", {}, "Scan of new images", "Scan neuer Images"],
  ["resource_action", { action: "check_environment_connection" }, "Connection check", "Verbindungsprüfung"],
  ["resource_action", {}, "Action", "Aktion"],
  ["resource_action", { operation: "container.shell" }, "Container shell", "Container-Shell"],
  ["job_run", { jobId: "environment-health:example" }, "Check host availability", "Host-Verfügbarkeit prüfen"],
  ["future_operation", {}, "Action: future operation", "Aktion: future operation"],
  ["job_run", { jobId: "future-job" }, "Action: future job", "Aktion: future job"],
  ["resource_action", { action: "future_action" }, "Action: future action", "Aktion: future action"],
  ["job_run", { jobId: "volume-backup:example" }, "Volume backup", "Volume-Sicherung"],
] as const)("renders producer-shaped %s titles in both catalogues", (type, metadata, en, de) => {
  const coded = mapped(type, metadata);
  expect(renderRun(coded, "en-US").title).toBe(en);
  expect(renderRun(coded, "de").title).toBe(de);
  expect(renderRun(coded, "de").target).toBe("");
  expect(coded.hostScoped).toBe(true);
});
it("keeps repository-check backup precedence and behaviour", () => {
  const coded = mapped("resource_action", { action: "check_volume_backup_repository" }, "backup_target");
  expect(coded.kind).toBe("repository-check");
  expect(renderRun(coded, "en-US").title).toBe("Repository check");
  expect(renderRun(coded, "de").title).toBe("Repository-Prüfung");
});
it.each([
  ["create_volume_backup", "Volume backup"],
  ["delete_volume_backup", "Delete volume backup"],
  ["download_volume_backup", "Download volume backup"],
  ["restore_volume_backup", "Restore volume backup"],
  ["update_volume_backup_policy", "Update backup policy"],
])("names the backup action %s while retaining its behaviour", (action, expected) => {
  const coded = mapped("resource_action", { action }, "volume");
  expect(coded.kind).toBe(action.startsWith("restore_") ? "volume-restore" : "backup");
  expect(renderRun(coded, "en-US").title).toBe(expected);
});
it("has complete renderable EN/DE entries for every explicit operation mapping", () => {
  for (const [type, metadata] of [
    ...Object.keys(TYPE_TITLES).map((type) => [type, {}] as const),
    ...Object.keys(JOB_TITLES).map((jobId) => ["job_run", { jobId }] as const),
    ...Object.keys(JOB_PREFIX_TITLES).map((prefix) => ["job_run", { jobId: `${prefix}example` }] as const),
    ...Object.keys(ACTION_TITLES).map((action) => ["resource_action", { action }] as const),
  ]) for (const locale of ["en-US", "de"] as const) {
    const value = renderRun(mapped(type, metadata), locale).title;
    expect(value).not.toMatch(/^(runMessages|activity)\./);
    expect(value).not.toMatch(/^(Activity|Aktivität)$/);
  }
});

it("titles a system backup resource action from its producer shape in both locales", () => {
  const run = mapped("resource_action", {}, "system_backup");
  expect(renderRun(run, "en-US").title).toBe("System backup");
  expect(renderRun(run, "de").title).toBe("Systemsicherung");
});

it("renders the real manual container-check producer with its host", async () => {
 const fixture = JSON.parse(readFileSync(join(root, "web/src/features/containers/testdata/manual-update-activity.json"), "utf8"));
 const run = mapActivityDetail({ activity: fixture, messages: [], output: [], stepTrace: [], outputTruncated: false });
 const rendered = renderRun(run, "en-US");
 expect(run.hostId).toBe("example-host");
 expect(rendered.title).toBe("Update check");
});
