// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useId, type ReactNode } from "react";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { ToneBadge } from "@/shared/components/tone";

import { useProxyText } from "./use-proxy-text";
export function TextField({ label, value, onChange, hint, type = "text", required, placeholder }: { label: string; value: string; onChange: (value: string) => void; hint?: string; type?: string; required?: boolean; placeholder?: string }) {
  const id = useId();
  return <Field><FieldLabel htmlFor={id}>{label}</FieldLabel><Input id={id} value={value} onChange={(e) => onChange(e.target.value)} type={type} required={required} placeholder={placeholder} />{hint && <FieldDescription>{hint}</FieldDescription>}</Field>;
}
export function Choice({ label, value, onChange, options, className }: { label: string; value: string; onChange: (value: string) => void; options: { value: string; label: string }[]; className?: string }) {
  return <Select value={value} onValueChange={onChange}><SelectTrigger aria-label={label} className={className}><SelectValue placeholder={label} /></SelectTrigger><SelectContent><SelectGroup>{options.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectGroup></SelectContent></Select>;
}
export function SelectField(props: Parameters<typeof Choice>[0]) {
  return <Field><FieldLabel>{props.label}</FieldLabel><Choice {...props} className="w-full" /></Field>;
}
export function Detail({ label, children }: { label: string; children: ReactNode }) {
  return <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1 border-b border-border/60 py-3 text-sm last:border-0"><dt className="text-muted-foreground">{label}</dt><dd className="min-w-0 break-words text-right">{children}</dd></div>;
}
export function RouteStatus({ enabled }: { enabled: boolean }) {
  const t = useProxyText();
  return <ToneBadge tone={enabled ? "ok" : "gray"}>{t(enabled ? "active" : "disabled")}</ToneBadge>;
}

const SECTION_TONES = {
  domain: "border-cyan/25 bg-cyan/5", target: "border-primary/25 bg-primary/5",
  tls: "border-success/25 bg-success/5", behavior: "border-border bg-muted/20",
} as const;
export function FormSection({ title, tone, children }: { title: string; tone: keyof typeof SECTION_TONES; children: ReactNode }) {
  return <fieldset className={"min-w-0 rounded-xl border p-4 " + SECTION_TONES[tone]}><legend className="px-2 text-sm font-semibold">{title}</legend><FieldGroup className="gap-4">{children}</FieldGroup></fieldset>;
}
export function OptionCards({ label, value, onChange, options }: { label: string; value: string; onChange: (value: string) => void; options: { value: string; label: string; description?: string }[] }) {
  const id = useId();
  return <Field><FieldLabel>{label}</FieldLabel><RadioGroup aria-label={label} value={value} onValueChange={onChange} className="grid gap-2 sm:grid-cols-2">{options.map((option) => <label key={option.value} htmlFor={id + option.value} className="flex cursor-pointer items-start gap-3 rounded-lg border bg-card p-3 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary/10"><RadioGroupItem id={id + option.value} value={option.value} className="mt-0.5" /><span className="min-w-0"><span className="block text-sm font-medium">{option.label}</span>{option.description && <span className="mt-1 block text-xs text-muted-foreground">{option.description}</span>}</span></label>)}</RadioGroup></Field>;
}
