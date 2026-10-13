import { useTranslations } from "use-intl";

import type { ParseError } from "@/domain/compose/model";
import type { ComposeEditError } from "@/domain/compose/ops";
import type { Issue } from "@/domain/compose/validate";

import type { EditorDiagnostic } from "@/shared/workspace/code-editor";

/** Translates validation codes and parse errors into editor texts. */
export function useEditorText() {
  const t = useTranslations("editor");
  const issue = (i: Issue) => t(`issue.${i.code}`, { ref: i.ref ?? "" });
  const parseError = (e: ParseError) => {
    const known = e.message === "root-not-map" || e.message === "services-not-map";
    const msg = known ? t(`errors.${e.message as "root-not-map"}`) : e.message;
    return e.line ? `${t("raw.line", { line: e.line })}: ${msg}` : msg;
  };
  const editError = (e: ComposeEditError) => t(`errors.${e.code}`, { name: e.detail ?? "" });
  return { issue, parseError, editError };
}

/** Places service-level issues on the service key line of the raw text. */
export function issueLine(compose: string, service: string | undefined): number {
  if (!service) return 1;
  const lines = compose.split("\n");
  const escaped = service.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const re = new RegExp(`^\\s{1,4}["']?${escaped}["']?:\\s*$`);
  const idx = lines.findIndex((l) => re.test(l));
  return idx >= 0 ? idx + 1 : 1;
}

export function toDiagnostics(compose: string, parseErrors: ParseError[], issues: Issue[], issueText: (i: Issue) => string, parseText: (e: ParseError) => string): EditorDiagnostic[] {
  if (parseErrors.length) return parseErrors.map((e) => ({ line: e.line ?? 1, message: parseText(e), severity: "error" as const }));
  return issues.map((i) => ({ line: issueLine(compose, i.service), message: (i.service ? `${i.service}: ` : "") + issueText(i), severity: i.level === "error" ? ("error" as const) : ("warning" as const) }));
}
