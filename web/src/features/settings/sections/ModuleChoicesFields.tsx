// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original shared setup module switches.

import { useTranslations } from "use-intl";
import { MODULES, type ModuleChoices } from "@/domain/modules";
import { Switch } from "@/shared/ui/switch";
import { Field, FieldGroup, FieldLabel } from "@/shared/ui/field";

export function ModuleChoicesFields({ value, onChange }: { value: ModuleChoices; onChange: (value: ModuleChoices) => void }) {
  const t = useTranslations("settings.modules");
  return <FieldGroup>{MODULES.map((m) => <Field key={m.id} orientation="horizontal" className="justify-between rounded-xl border p-4">
    <FieldLabel htmlFor={`module-${m.id}`}>{t(`names.${m.id}`)}</FieldLabel>
    <Switch id={`module-${m.id}`} checked={value[m.id]} onCheckedChange={(enabled) => onChange({ ...value, [m.id]: enabled })} />
  </Field>)}</FieldGroup>;
}
