import { useState } from "react";
import { useTranslations } from "use-intl";

import { formatArgs, parseArgs } from "@/domain/compose/health-presets";
import type { HealthModel } from "@/domain/compose/model";
import { ops, type Mutator } from "@/domain/compose/ops";
import { Badge } from "@/shared/ui/badge";
import { Field, FieldDescription, FieldError, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";

interface Props {
  serviceKey: string;
  health: HealthModel;
  inputId: string;
  /** Validation text for a missing test, already translated. */
  issue?: string;
  edit: (fn: Mutator) => void;
}

/**
 * Test command in the file's own form. CMD is edited as a JSON list so every
 * argument stays exact; text that is not a list of strings is never written.
 */
export function HealthTestField({ serviceKey, health: h, inputId, issue, edit }: Props) {
  const t = useTranslations("editor.services.health");
  const hintId = `${inputId}-form`;
  const exec = h.form === "CMD";
  const fileArgs = formatArgs(h.args);
  const [text, setText] = useState(fileArgs);
  const [synced, setSynced] = useState(fileArgs);
  // Follow outside changes (preset, Advanced editor) unless the field already says the same.
  if (fileArgs !== synced) {
    setSynced(fileArgs);
    const own = parseArgs(text);
    if (!own || formatArgs(own) !== fileArgs) setText(fileArgs);
  }
  const jsonError = exec && !parseArgs(text);
  const error = jsonError ? t("argsInvalid") : issue;
  return (
    <Field data-invalid={error ? true : undefined}>
      <div className="flex flex-wrap items-center gap-2">
        <FieldLabel htmlFor={inputId}>{exec ? t("argsLabel") : t("test")}</FieldLabel>
        <Badge variant="outline" className="h-5 px-1.5 font-mono text-[10.5px]" title={t(`formHint.${h.form}`)}>
          {t(`form.${h.form}`)}
        </Badge>
      </div>
      {exec ? (
        <Input
          id={inputId}
          className="font-mono"
          spellCheck={false}
          placeholder={t("argsPlaceholder")}
          aria-invalid={error ? true : undefined}
          aria-describedby={hintId}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            const args = parseArgs(e.target.value);
            if (args) edit(ops.setHealthArgs(serviceKey, args));
          }}
        />
      ) : (
        <Input id={inputId} className="font-mono" placeholder={t("testPlaceholder")} aria-invalid={error ? true : undefined} aria-describedby={hintId} value={h.test} onChange={(e) => edit(ops.setHealthTest(serviceKey, e.target.value, h.form === "string" ? "string" : "CMD-SHELL"))} />
      )}
      <FieldDescription id={hintId}>{t(`formHint.${h.form}`)}</FieldDescription>
      {error && <FieldError>{error}</FieldError>}
    </Field>
  );
}
