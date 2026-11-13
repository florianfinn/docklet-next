import type { ActionRun } from "@/domain/model";

import { collapseOutput } from "./output-collapse";

const clocks = new Map<string, Intl.DateTimeFormat>();

/** Wall-clock time with milliseconds in the hub zone, as raw CLI logs show it. */
export function outputTime(iso: string, timeZone: string): string {
  let clock = clocks.get(timeZone);
  if (!clock) {
    clock = new Intl.DateTimeFormat("en-GB", { timeZone, hour: "2-digit", minute: "2-digit", second: "2-digit", fractionalSecondDigits: 3, hourCycle: "h23" });
    clocks.set(timeZone, clock);
  }
  const part = (type: Intl.DateTimeFormatPartTypes) => clock.formatToParts(new Date(iso)).find((p) => p.type === type)?.value ?? "00";
  return `${part("hour")}:${part("minute")}:${part("second")}.${part("fractionalSecond")}`;
}

/** Plain-text export as a terminal shows it: chronological, layer progress folded to its newest frame. */
export function outputText(run: Pick<ActionRun, "title" | "output">): string {
  const rows = collapseOutput(run.output);
  return [`# ${run.title}`, ...rows.map(({ line: l }) => `${l.at} [${l.stream}]${l.level ? ` [${l.level}]` : ""} ${l.stream === "cmd" ? `$ ${l.text}` : l.text}`)].join("\n");
}
