import type { ActionRun } from "@/domain/model";

/** Static, clearly fictitious run for design previews; never part of the action list. */
export function sampleRun(startedAt: string): ActionRun {
  return {
    id: "preview-run",
    kind: "redeploy",
    title: "Pull & Redeploy · Beispiel",
    target: "Beispiel · Host Atlas",
    services: ["web", "worker"],
    status: "running",
    progress: 40,
    startedAt,
    steps: [
      { key: "validate", label: "Prüfung", state: "done" },
      { key: "pull", label: "Image-Pull", state: "done" },
      { key: "create", label: "Austausch", detail: "2 Container werden ersetzt", state: "running" },
      { key: "start", label: "Start", state: "pending" },
      { key: "health", label: "Zustandsprüfung", state: "pending" },
    ],
    log: [],
    output: [],
  };
}
