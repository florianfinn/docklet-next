// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/shared/components/form-field.tsx
// Revision: f36dbc66306b9720d9f01a67efc455798b2229328478b21d7b29c986cfdee58e
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: None; byte-identical L4 snapshot. See web/PROVENANCE.md. Simple-user management, assigned-stack access and mode-aware self-service.

import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";
import { Field, FieldContent, FieldDescription, FieldError, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { HelpLabel, type HelpKey } from "@/shared/help/help-tooltip";

import { PasswordInput } from "./password-input";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";

interface TextFieldProps {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  description?: ReactNode;
  /** Shown only after the field was touched or a submit was attempted. */
  error?: string;
  placeholder?: string;
  mono?: boolean;
  type?: "text" | "password" | "email" | "url" | "number" | "date";
  autoComplete?: string;
  disabled?: boolean;
  className?: string;
  inputClassName?: string;
  trailing?: ReactNode;
  /** Extended explanation shown by an info button beside the label. */
  help?: HelpKey;
}

/** Labelled input with description and an accessible error message. */
export function TextField({ id, label, value, onChange, description, error, placeholder, mono, type = "text", autoComplete, disabled, className, inputClassName, trailing, help }: TextFieldProps) {
  const descId = description ? `${id}-desc` : undefined;
  const errId = error ? `${id}-err` : undefined;
  const inputProps = {
    id,
    value,
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value),
    placeholder,
    autoComplete: autoComplete ?? "off",
    spellCheck: false,
    disabled,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": [descId, errId].filter(Boolean).join(" ") || undefined,
  };
  return (
    <Field data-invalid={error ? true : undefined} className={className}>
      {help ? (
        <HelpLabel htmlFor={id} topic={help}>
          {label}
        </HelpLabel>
      ) : (
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
      )}
      <div className="flex items-center gap-2">
        {type === "password" ? (
          <PasswordInput {...inputProps} className={inputClassName} />
        ) : (
          <Input {...inputProps} type={type} className={cn(mono && "font-mono text-[13px]", inputClassName)} />
        )}
        {trailing}
      </div>
      {description && <FieldDescription id={descId}>{description}</FieldDescription>}
      {error && <FieldError id={errId}>{error}</FieldError>}
    </Field>
  );
}

export interface ChoiceOption<T extends string> {
  value: T;
  title: ReactNode;
  description: ReactNode;
  icon?: ReactNode;
  badge?: ReactNode;
}

/** Radio group rendered as selectable cards (connection or login modes). */
export function ChoiceCards<T extends string>({ value, onChange, options, label, columns = 2 }: { value: T; onChange: (v: T) => void; options: ChoiceOption<T>[]; label: string; columns?: 2 | 3 }) {
  return (
    <RadioGroup value={value} onValueChange={(v) => onChange(v as T)} aria-label={label} className={cn("grid gap-3", columns === 3 ? "md:grid-cols-3" : "sm:grid-cols-2")}>
      {options.map((o) => {
        const id = `choice-${label.replace(/\W+/g, "-")}-${o.value}`;
        const active = o.value === value;
        return (
          <label
            key={o.value}
            htmlFor={id}
            // A row of equal cards: the hovered card keeps the edge light, its neighbour never takes it.
            data-glow="row"
            className={cn(
              "lift relative flex cursor-pointer gap-3 rounded-xl border bg-card/50 p-3.5 transition-colors has-focus-visible:ring-[3px] has-focus-visible:ring-ring/50",
              active ? "border-control/60 bg-control/12" : "hover:border-border hover:bg-muted/30",
            )}
          >
            <RadioGroupItem id={id} value={o.value} className="mt-0.5" />
            <FieldContent className="gap-1">
              <span className="flex items-center gap-2 text-sm font-medium">
                {o.icon && <span className={cn("[&_svg]:size-4", active ? "text-primary-text" : "text-muted-foreground")}>{o.icon}</span>}
                {o.title}
                {o.badge}
              </span>
              <span className="text-xs leading-relaxed text-muted-foreground">{o.description}</span>
            </FieldContent>
          </label>
        );
      })}
    </RadioGroup>
  );
}
