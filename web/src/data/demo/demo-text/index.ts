import type { AppLocale } from "@/domain/locale";
import type { ActionRun, ContainerInfo, OpsEvent, ScheduledJob, SecurityOverview } from "@/domain/model";

import { deContainers } from "@/features/containers/messages/de";
import { enContainers } from "@/features/containers/messages/en";

import { EXACT, PATTERNS } from "./en-us";

const REFUSALS = Object.fromEntries(Object.entries(deContainers.operationReasons).map(([key, text]) => [text, enContainers.operationReasons[key as keyof typeof enContainers.operationReasons]]));
const SEPARATORS = [" · ", " – "];

function translate(text: string): string {
  const exact = REFUSALS[text] ?? EXACT[text];
  if (exact !== undefined) return exact;
  for (const [re, fn] of PATTERNS) {
    const m = text.match(re);
    if (m) return fn(m, translate);
  }
  // "label – detail" or "title · target": head and rest translate on their own.
  for (const sep of SEPARATORS) {
    const at = text.indexOf(sep);
    if (at < 0) continue;
    const out = `${translate(text.slice(0, at))}${sep}${translate(text.slice(at + sep.length))}`;
    if (out !== text) return out;
  }
  return text;
}

/**
 * Text written by the demo backend, shown in the interface language. German
 * returns the stored text unchanged; unknown texts (names, raw values) pass through.
 */
export function localizeDemoText(text: string, locale: AppLocale): string;
export function localizeDemoText(text: string | undefined, locale: AppLocale): string | undefined;
export function localizeDemoText(text: string | undefined, locale: AppLocale): string | undefined {
  if (text === undefined || locale === "de") return text;
  return translate(text);
}

const runCache: Partial<Record<AppLocale, WeakMap<ActionRun, ActionRun>>> = {};

/** Run with its demo texts in the interface language; the same input returns the same object. */
export function localizeRun(run: ActionRun, locale: AppLocale): ActionRun {
  if (locale === "de") return run;
  const cache = (runCache[locale] ??= new WeakMap());
  let out = cache.get(run);
  if (!out) {
    out = {
      ...run,
      title: translate(run.title),
      target: translate(run.target),
      error: localizeDemoText(run.error, locale),
      steps: run.steps.map((s) => ({ ...s, label: translate(s.label), detail: localizeDemoText(s.detail, locale) })),
      log: run.log.map(translate),
      output: run.output.map((l) => (l.stream === "note" ? { ...l, text: translate(l.text) } : l)),
    };
    cache.set(run, out);
  }
  return out;
}

/** Event titles in the interface language. */
export function localizeEvents(events: OpsEvent[], locale: AppLocale): OpsEvent[] {
  return events.map((e) => ({ ...e, title: localizeDemoText(e.title, locale) }));
}

/** Scheduled job titles and cadences in the interface language. */
export function localizeSchedule(jobs: ScheduledJob[], locale: AppLocale): ScheduledJob[] {
  return jobs.map((j) => ({ ...j, title: localizeDemoText(j.title, locale), cadence: localizeDemoText(j.cadence, locale) }));
}

/** Synthetic finding titles in the interface language; ids and packages stay. */
export function localizeSecurity(security: SecurityOverview, locale: AppLocale): SecurityOverview {
  return { ...security, findings: security.findings.map((f) => ({ ...f, title: localizeDemoText(f.title, locale) })) };
}

/** Origin notes in the interface language. */
export function localizeContainers(containers: ContainerInfo[], locale: AppLocale): ContainerInfo[] {
  return containers.map((c) => (c.originNote ? { ...c, originNote: localizeDemoText(c.originNote, locale) } : c));
}
