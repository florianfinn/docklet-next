import { describe, expect, it } from "vitest";

import type { ActionRun, RunOutputLine } from "@/domain/model";
import { mergeUiStyle, DEFAULT_UI_STYLE } from "@/shared/preferences/ui-style-store";

import { offerRun, showRun, type ActionDialogState } from "./action-dialog-store";
import { isMajorAction } from "./action-policy";
import { deployRuns, shownRun } from "./deploy-history";
import { collapseOutput } from "./output-collapse";
import { outputText } from "./run-output-text";

const at = (s: number) => new Date(Date.UTC(2026, 9, 11, 12, 0, s)).toISOString();
const line = (s: number, stream: RunOutputLine["stream"], text: string, step: RunOutputLine["step"] = "pull"): RunOutputLine => ({ at: at(s), stream, text, step });

describe("raw output folding", () => {
  const lines = [
    line(0, "cmd", "docker compose --project-name shop pull web"),
    line(1, "stderr", " web Pulling"),
    line(2, "stderr", " 0123456789ab Pulling fs layer"),
    line(3, "stderr", " fedcba987654 Pulling fs layer"),
    line(4, "stderr", " 0123456789ab Downloading [====>    ]  1.0MB/3.4MB"),
    line(5, "stderr", " fedcba987654 Downloading [==>      ]  2.0MB/28.7MB"),
    line(6, "stderr", " 0123456789ab Downloading [========>]  3.0MB/3.4MB"),
    line(7, "stderr", " 0123456789ab Pull complete"),
    line(8, "stderr", "Error response from daemon: pull failed", "pull"),
    line(9, "cmd", "docker pull nginx:1.27"),
    line(10, "stderr", " 0123456789ab Already exists"),
  ];

  it("keeps one updating row per layer at its first position, like a terminal", () => {
    const rows = collapseOutput(lines);
    expect(rows.map((r) => [r.line.text.trim(), r.frames])).toEqual([
      ["docker compose --project-name shop pull web", 1],
      ["web Pulling", 1],
      ["0123456789ab Pull complete", 4],
      ["fedcba987654 Downloading [==>      ]  2.0MB/28.7MB", 2],
      ["Error response from daemon: pull failed", 1],
      ["docker pull nginx:1.27", 1],
      // A new command starts its own terminal region.
      ["0123456789ab Already exists", 1],
    ]);
    expect(rows[2]!.firstAt).toBe(at(2));
  });

  it("sorts chronologically and exports the folded view with timestamps and streams", () => {
    const text = outputText({ title: "Pull", output: [...lines].reverse() });
    const body = text.split("\n").slice(1);
    expect(body.length).toBe(7);
    expect(body[0]!.startsWith(`${at(0)} [cmd] $ docker compose`)).toBe(true);
    expect(body.filter((l) => l.includes("0123456789ab")).length).toBe(2);
  });
});

describe("action dialog coordination", () => {
  const closed: ActionDialogState = { waiting: [] };

  it("opens for the first major run but never replaces a shown run by itself", () => {
    const first = offerRun(closed, "a");
    const second = offerRun(first, "b");
    expect([first.runId, second.runId, second.waiting]).toEqual(["a", "a", ["b"]]);
    expect(offerRun(second, "b") === second && offerRun(second, "a") === second).toBe(true);
  });

  it("switches only on a deliberate open and drops that run from the offers", () => {
    const state = showRun({ runId: "a", waiting: ["b", "c"] }, "c");
    expect(state).toEqual({ runId: "c", waiting: ["b"] });
  });
});

describe("major action policy", () => {
  it("covers deploy, update, restore, pull and backups but not starts, checks or scans", () => {
    const major = (["deploy", "redeploy", "update", "restore", "volume-restore", "pull", "backup", "prune", "recreate"] as const).every((kind) => isMajorAction({ kind }));
    const minor = (["start", "stop", "restart", "check-updates", "scan", "reconnect", "agent-restart", "onboard"] as const).some((kind) => isMajorAction({ kind }));
    expect([major, minor]).toEqual([true, false]);
  });
});

describe("deploy history selection", () => {
  const run = (id: string, s: number, kind: ActionRun["kind"] = "redeploy", stackId = "shop"): ActionRun => ({
    id,
    kind,
    title: id,
    target: "Shop",
    stackId,
    services: [],
    steps: [],
    status: "succeeded",
    progress: 100,
    startedAt: at(s),
    log: [],
    output: [],
  });

  it("merges live and recorded runs once, newest first, deploy kinds of this stack only", () => {
    const runs = deployRuns([run("new", 30), run("old", 10), run("chk", 40, "check-updates"), run("other", 50, "redeploy", "media")], [run("old", 10), run("seed", 5)], "shop");
    expect(runs.map((r) => r.id)).toEqual(["new", "old", "seed"]);
  });

  it("shows a picked run until a newer run starts, then follows the new run", () => {
    const runs = [run("b", 20), run("a", 10)];
    const pick = { runId: "a", latestId: "b" };
    expect(shownRun(runs, pick)?.id).toBe("a");
    expect(shownRun([run("c", 30), ...runs], pick)?.id).toBe("c");
    expect(shownRun(runs, null)?.id).toBe("b");
  });
});

describe("notification preferences", () => {
  it("fall back to defaults for unknown designs and keep valid ones", () => {
    const merged = mergeUiStyle({ toastDesign: "boxes", actionDialogDesign: "console", actionPresentationOverride: "yes" }, DEFAULT_UI_STYLE);
    expect([merged.toastDesign, merged.actionDialogDesign, merged.actionPresentationOverride]).toEqual(["bar", "console", "hub"]);
  });
});
