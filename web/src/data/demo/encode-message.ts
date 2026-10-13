import type { MessageCode } from "../message-code";
import type { CodedActionRun, CodedEvent, CodedStackDetail } from "../message-code";
import type { ActionRun, OpsEvent, StackDetail } from "@/domain/model";

import { PATTERNS } from "./demo-text/en-us";
import { localizeDemoText } from "./demo-text";
import { EXACT_CODES, PATTERN_CODES } from "./message-codes";

const nested = new Set([18, 27, 43, 66, 68, 69, 70]);
const plural = new Set([0, 1, 2, 3, 4, 8, 39, 40, 56, 57]);

/** Convert demo prose to the same code and parameter shape used by a backend. */
export function encodeDemoMessage(text: string): MessageCode {
  if (Object.hasOwn(EXACT_CODES, text)) return { code: `runMessages.exact.${EXACT_CODES[text]}`, params: {} };
  for (let i = 0; i < PATTERN_CODES.length; i++) {
    const match = text.match(PATTERNS[i]![0]);
    if (!match) continue;
    let code = `runMessages.pattern.${PATTERN_CODES[i]}`;
    if (i === 41 || i === 60 || i === 64) code += match[i === 60 ? 3 : 2] ? "long" : "short";
    const params: MessageCode["params"] = {};
    for (let j = 1; j < match.length; j++) {
      if (match[j] === undefined) continue;
      params[`p${j}`] = nested.has(i) ? encodeDemoMessage(match[j]!) : match[j]!;
    }
    if (i === 33) params.p1 = Number(match[1]!.replace(",", "."));
    if (plural.has(i)) params.n1 = Number(match[1]);
    return { code, params };
  }
  for (const [separator, key] of [[" · ", "dot"], [" – ", "dash"]] as const) {
    const at = text.indexOf(separator);
    if (at < 0) continue;
    return { code: `runMessages.join.${key}`, params: { left: encodeDemoMessage(text.slice(0, at)), right: encodeDemoMessage(text.slice(at + separator.length)) } };
  }
  if (localizeDemoText(text, "en-US") !== text) throw new Error("Unmapped demo message");
  return { code: "runMessages.value", params: { value: text } };
}

const runCache = new WeakMap<ActionRun, CodedActionRun>();
const runsCache = new WeakMap<ActionRun[], CodedActionRun[]>();

export function encodeDemoRun(run: ActionRun): CodedActionRun {
  const existing = runCache.get(run);
  if (existing) return existing;
  const coded: CodedActionRun = {
    ...run,
    title: encodeDemoMessage(run.title),
    target: encodeDemoMessage(run.target),
    error: run.error === undefined ? undefined : encodeDemoMessage(run.error),
    steps: run.steps.map((step) => ({ ...step, label: encodeDemoMessage(step.label), detail: step.detail === undefined ? undefined : encodeDemoMessage(step.detail) })),
    log: run.log.map(encodeDemoMessage),
    output: run.output.map((line) => ({ ...line, text: line.stream === "note" ? encodeDemoMessage(line.text) : line.text })),
  };
  runCache.set(run, coded);
  return coded;
}

export function encodeDemoRuns(runs: ActionRun[]): CodedActionRun[] {
  const existing = runsCache.get(runs);
  if (existing) return existing;
  const coded = runs.map(encodeDemoRun);
  runsCache.set(runs, coded);
  return coded;
}

export function encodeDemoEvent(event: OpsEvent): CodedEvent {
  return { ...event, title: encodeDemoMessage(event.title) };
}

export function encodeDemoStackDetail(stack: StackDetail): CodedStackDetail {
  return { ...stack, deployments: stack.deployments.map(encodeDemoRun) };
}
