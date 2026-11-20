import { createTranslator } from "use-intl";

import { wireKey } from "./wire-catalogue";
import { MESSAGE_CODE_KEYS } from "./wire-catalogue-messages";

import { catalogs } from "@/app/i18n/catalog";
import type { AppLocale } from "@/domain/locale";
import type { ActionRun, ActionStep, OpsEvent, RunOutputLine, StackDetail } from "@/domain/model";

export interface MessageCode {
  code: string;
  params: Record<string, string | number | MessageCode>;
  sourceText?: string;
}

export type CodedStep = Omit<ActionStep, "label" | "detail"> & { label: MessageCode; detail?: MessageCode };
export type CodedOutputLine = Omit<RunOutputLine, "text"> & { text: string | MessageCode };
export type CodedActionRun = Omit<ActionRun, "title" | "target" | "error" | "steps" | "log" | "peerAudit" | "output"> & {
  title: MessageCode;
  backupOutcome?: { status: string; errorCode?: string; reason?: string };
  target: MessageCode;
  error?: MessageCode;
  steps: CodedStep[];
  log: MessageCode[];
  peerAudit?: MessageCode;
  output: CodedOutputLine[];
};
export type CodedEvent = Omit<OpsEvent, "title"> & { title: MessageCode };
export type CodedStackDetail = Omit<StackDetail, "deployments"> & { deployments: CodedActionRun[] };

const translators = {
  de: createTranslator({ locale: "de", messages: catalogs.de }),
  "en-US": createTranslator({ locale: "en-US", messages: catalogs["en-US"] }),
};

const GUARDED_MESSAGE_KEYS = new Set<string>(Object.values(MESSAGE_CODE_KEYS));

/** The adapter sends codes; display text is resolved at the UI boundary. */
export function renderMessage(message: MessageCode, locale: AppLocale): string {
  const guardedKey = wireKey(MESSAGE_CODE_KEYS, message.code) ?? (GUARDED_MESSAGE_KEYS.has(message.code) ? message.code : undefined);
  const values: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(message.params)) {
    values[key] = typeof value === "object" ? renderMessage(value, locale) : value;
  }
  const translate = translators[locale] as unknown as (key: string, values: Record<string, string | number>) => string;
  if (message.code === "runtime.peer-acknowledged") {
 const count = Number(values.count);
 const peers = Array.from({ length: Math.min(count,16) }, (_,i) => values[`peer${i}`]).filter((v): v is string => typeof v === "string");
 values.peers = peers.join(", ");
 if (count > 16) values.peers += `; ${translate("common.runtimePeers.more", { count: count - 16 })}`;
 }
 const key = guardedKey ?? message.code;
  if (!guardedKey && !translators[locale].has(key as never)) return message.sourceText ?? message.code;
  try {
    return translate(key, values);
  } catch (error) {
    if (!guardedKey && message.sourceText !== undefined) return message.sourceText;
    throw error;
  }
}

const runCache: Partial<Record<AppLocale, WeakMap<CodedActionRun, ActionRun>>> = {};

export function renderRun(run: CodedActionRun, locale: AppLocale): ActionRun {
  const cache = (runCache[locale] ??= new WeakMap());
  const existing = cache.get(run);
  if (existing) return existing;
  const result: ActionRun = {
    ...run,
    title: renderMessage(run.title, locale),
    target: renderMessage(run.target, locale),
    error: run.error === undefined ? undefined : renderMessage(run.error, locale),
    steps: run.steps.map((step) => ({ ...step, label: renderMessage(step.label, locale), detail: step.detail === undefined ? undefined : renderMessage(step.detail, locale) })),
    log: run.log.map((line) => renderMessage(line, locale)),
    peerAudit: run.peerAudit === undefined ? undefined : renderMessage(run.peerAudit, locale),
    output: run.output.map((line) => ({ ...line, text: typeof line.text === "string" ? line.text : renderMessage(line.text, locale) })),
  };
  cache.set(run, result);
  return result;
}

export function renderEvent(event: CodedEvent, locale: AppLocale): OpsEvent {
  return { ...event, title: renderMessage(event.title, locale) };
}

export function renderStackDetail(stack: CodedStackDetail, locale: AppLocale): StackDetail {
  return { ...stack, deployments: stack.deployments.map((run) => renderRun(run, locale)) };
}
