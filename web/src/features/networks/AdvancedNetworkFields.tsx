// Source: docklet-next/issues/204
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original advanced network editors using existing free UI primitives.

import { ChevronDown, Plus, Trash2 } from "lucide-react";
import { useTranslations } from "use-intl";

import type { NetworkAuxAddress } from "@/domain/model";
import type { NetworkDraft, NetworkDraftErrors } from "@/domain/networks";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";
import { FieldGroup } from "@/shared/ui/field";

export interface NetworkEntry { key: string; value: string }

type Section = "labels" | "options" | "auxAddresses" | "auxAddresses6";

function EntryEditor({ section, entries, onChange, error }: { section: Section; entries: NetworkEntry[]; onChange: (entries: NetworkEntry[]) => void; error?: string }) {
  const t = useTranslations("networks.createDialog.advanced");
  const aux = section === "auxAddresses" || section === "auxAddresses6";
  const patch = (index: number, change: Partial<NetworkEntry>) => onChange(entries.map((entry, i) => i === index ? { ...entry, ...change } : entry));
  return (
    <fieldset className="flex min-w-0 flex-col gap-3" aria-describedby={error ? `${section}-error` : undefined}>
      <legend className="mb-2 text-sm font-medium">{t(section)}</legend>
      {entries.map((entry, index) => (
        <div key={index} className="flex min-w-0 items-start gap-2" data-slot="network-entry-row">
          <FieldGroup className="min-w-0 flex-1 gap-2 sm:grid sm:grid-cols-2">
            <TextField id={`${section}-${index}-key`} label={t(aux ? "name" : "key")} value={entry.key} onChange={(key) => patch(index, { key })} error={error} mono inputClassName="min-w-0" />
            <TextField id={`${section}-${index}-value`} label={t(aux ? "address" : "value")} value={entry.value} onChange={(value) => patch(index, { value })} error={error} mono inputClassName="min-w-0" />
          </FieldGroup>
          <Button type="button" variant="ghost" size="icon" className="mt-6 shrink-0" aria-label={t("remove", { section: t(section), index: index + 1 })} onClick={() => onChange(entries.filter((_, i) => i !== index))}>
            <Trash2 data-icon="inline-start" />
          </Button>
        </div>
      ))}
      {error && <p id={`${section}-error`} role="alert" className="text-sm text-destructive">{error}</p>}
      <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => onChange([...entries, { key: "", value: "" }])}>
        <Plus data-icon="inline-start" /> {t("add", { section: t(section) })}
      </Button>
    </fieldset>
  );
}

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  draft: NetworkDraft;
  patch: (patch: Partial<NetworkDraft>) => void;
  labels: NetworkEntry[];
  options: NetworkEntry[];
  onLabelsChange: (entries: NetworkEntry[]) => void;
  onOptionsChange: (entries: NetworkEntry[]) => void;
  error: (key: keyof NetworkDraftErrors) => string | undefined;
}

export function AdvancedNetworkFields({ open, onOpenChange, draft, patch, labels, options, onLabelsChange, onOptionsChange, error }: Props) {
  const t = useTranslations("networks.createDialog.advanced");
  const reservations = (section: "auxAddresses" | "auxAddresses6", entries: NetworkAuxAddress[] = []) => (
    <EntryEditor section={section} entries={entries.map((a) => ({ key: a.name, value: a.ip }))} onChange={(next) => patch({ [section]: next.map((e) => ({ name: e.key, ip: e.value })) })} error={error(section)} />
  );
  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger asChild>
        <Button type="button" variant="outline"><ChevronDown data-icon="inline-start" /> {t("title")}</Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-4">
        <FieldGroup className="min-w-0 gap-5">
          <p className="text-xs text-muted-foreground">{t("hint")}</p>
          <EntryEditor section="labels" entries={labels} onChange={onLabelsChange} error={error("labels")} />
          <EntryEditor section="options" entries={options} onChange={onOptionsChange} error={error("options")} />
          {draft.ipv4 !== false && reservations("auxAddresses", draft.auxAddresses)}
          {draft.ipv6 && reservations("auxAddresses6", draft.auxAddresses6)}
        </FieldGroup>
      </CollapsibleContent>
    </Collapsible>
  );
}
