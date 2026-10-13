import type { ActionRun } from "@/domain/model";

import { collapseOutput } from "./output-collapse";

const pad = (n: number, w = 2) => String(n).padStart(w, "0");

/** Wall-clock time with milliseconds, as raw CLI logs show it. */
export function outputTime(iso: string): string {
  const d = new Date(iso);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
}

/** Plain-text export as a terminal shows it: chronological, layer progress folded to its newest frame. */
export function outputText(run: Pick<ActionRun, "title" | "output">): string {
  const rows = collapseOutput(run.output);
  return [`# ${run.title}`, ...rows.map(({ line: l }) => `${l.at} [${l.stream}]${l.level ? ` [${l.level}]` : ""} ${l.stream === "cmd" ? `$ ${l.text}` : l.text}`)].join("\n");
}
