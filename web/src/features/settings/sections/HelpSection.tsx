import { useTranslations } from "use-intl";

import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { HELP_MODES, helpStore, type HelpPrefs } from "@/shared/help/help-store";
import { HelpBody, HelpLabel } from "@/shared/help/help-tooltip";
import { useStore } from "@/shared/lib/store";
import { Field, FieldDescription } from "@/shared/ui/field";

import { SettingsForm } from "../SettingsForm";
import { useSectionDraft } from "../use-section-draft";

/**
 * Detail level of the extended explanations. A personal reading preference,
 * so it is stored in this browser like the appearance; it follows the usual
 * draft with Save and Discard instead of applying on click.
 */
export function HelpSection() {
  const t = useTranslations("settings");
  const th = useTranslations("settings.help");
  const saved = useStore(helpStore, (s) => s);
  const section = useSectionDraft<HelpPrefs>(saved);
  const d = section.draft;
  if (!d) return null;
  return (
    <SettingsForm
      title={t("sections.help")}
      description={th("description")}
      dirty={section.dirty}
      onDiscard={section.reset}
      onSave={async () => {
        helpStore.set(d);
        section.commit(d);
      }}
      note={t("form.browserLocal")}
    >
      <Field>
        <HelpLabel id="help-mode-label" topic="settingsHelp.mode">
          {th("mode")}
        </HelpLabel>
        <SegmentedToggle value={d.mode} onChange={(mode) => section.patch({ mode })} label={th("mode")} options={HELP_MODES.map((m) => ({ value: m, label: th(`modes.${m}`) }))} className="self-start" />
        <FieldDescription>{th(`modeHints.${d.mode}`)}</FieldDescription>
      </Field>
      <section aria-labelledby="help-preview" className="flex flex-col gap-2">
        <h3 id="help-preview" className="text-sm font-medium">
          {th("preview")}
        </h3>
        {d.mode === "off" ? (
          <p className="text-xs text-muted-foreground">{th("previewOff")}</p>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">{th("previewTopic")}</p>
            <div data-slot="help-preview" className="max-w-80 overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-sm">
              <HelpBody topic="settingsGeneral.hubAddress" mode={d.mode} />
            </div>
          </>
        )}
        <p className="text-xs text-muted-foreground">{th("trigger")}</p>
      </section>
    </SettingsForm>
  );
}
