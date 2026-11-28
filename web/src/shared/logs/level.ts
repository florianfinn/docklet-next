// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import type { LogLevel } from "@/domain/model";

const aliases: Record<string, LogLevel> = { error: "error", fatal: "error", crit: "error", panic: "error", err: "error", e: "error", warn: "warn", warning: "warn", w: "warn", info: "info", i: "info", debug: "debug", trace: "debug", d: "debug", fail: "error" };
const level = (value: unknown) => typeof value === "string" ? aliases[value.toLowerCase()] : undefined;
/** Explicit structured severity wins over words in the message body. */
export function classifyLevel(message: string): LogLevel {
  try {
    const obj = JSON.parse(message);
    if (obj && typeof obj === "object" && !Array.isArray(obj)) {
      for (const key of ["level", "lvl", "severity", "log.level"]) {
        const found = level(obj[key]);
        if (found) return found;
      }
    }
  } catch { /* Most log messages are plain text. */ }
  const fmt = /(?:^|\s)(?:level|lvl)=(?:"([^"]+)"|'([^']+)'|([^\s]+))/.exec(message);
  const formatted = level(fmt?.[1] ?? fmt?.[2] ?? fmt?.[3]);
  if (formatted) return formatted;
  const body = message.trimStart().replace(/^\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?\s+/, "");
  const bracket = /^\[(error|warn|warning|info|debug|E|W|I|D)\](?:\s|$)/i.exec(body);
  const marker = /^(ERROR|WARN|INFO|DEBUG)\b/.exec(body) ?? /^(fail|warn|info):/i.exec(body);
  const explicit = level(bracket?.[1] ?? marker?.[1]);
  if (explicit) return explicit;
  if (/\b(error|fatal|panic|exception)\b/i.test(message)) return "error";
  if (/\b(warn|warning)\b/i.test(message)) return "warn";
  return "info";
}
