import { Moon, Sun } from "lucide-react";
import { useEffect } from "react";
import { useTranslations } from "use-intl";

import { labStore, setTokens, tokenPreviewStore, type DesignTokens } from "@/features/design-lab/design-tokens";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { useStore } from "@/shared/lib/store";
import { preferencesStore, type OverviewMode } from "@/shared/preferences/preferences-store";
import { setVisualStyle, sidebarWidthPreviewStore, useVisualStyle, VISUAL_RANGES } from "@/shared/preferences/visual-style-store";
import { PaletteEditor } from "@/shared/theme/PaletteEditor";
import { HelpLabel, HelpLegend } from "@/shared/help/help-tooltip";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet } from "@/shared/ui/field";
import { Slider } from "@/shared/ui/slider";

import { SettingsForm } from "../SettingsForm";
import { useSectionDraft } from "../use-section-draft";

const TOKEN_FIELDS = ["theme", "palette", "density", "motion"] as const;
type TokenField = (typeof TOKEN_FIELDS)[number];
type Appearance = Pick<DesignTokens, TokenField> & { stacksMode: OverviewMode; containersMode: OverviewMode; sidebarWidth: number };

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/** Token fields the draft changes compared with the saved state. */
function tokenChanges(draft: Appearance, base: Appearance): Partial<DesignTokens> {
  return Object.fromEntries(TOKEN_FIELDS.filter((k) => !same(draft[k], base[k])).map((k) => [k, draft[k]]));
}

/**
 * Personal appearance, saved into the browser-local token, preference and
 * visual style stores that the design lab uses as well. Edits show at once as a preview;
 * Discard or leaving the page restores the saved state. Saving writes only
 * changed fields, and fields changed elsewhere (lab, profile menu) replace
 * the matching draft field instead of being overwritten later.
 */
export function AppearanceSection() {
  const t = useTranslations("settings");
  const ta = useTranslations("settings.appearance");
  const tokens = useStore(labStore, (s) => s.tokens);
  const prefs = useStore(preferencesStore, (p) => p);
  const sidebarWidth = useVisualStyle("sidebarWidth");
  const saved: Appearance = { theme: tokens.theme, palette: tokens.palette, density: tokens.density, motion: tokens.motion, stacksMode: prefs.stacksMode, containersMode: prefs.containersMode, sidebarWidth };
  const { draft, patch, dirty, reset, commit } = useSectionDraft(saved, { rebase: "fields" });
  const preview = draft ? tokenChanges(draft, saved) : {};
  const previewKey = JSON.stringify(preview);

  useEffect(() => {
    tokenPreviewStore.set(previewKey === "{}" ? null : (JSON.parse(previewKey) as Partial<DesignTokens>));
  }, [previewKey]);
  const widthPreview = draft && draft.sidebarWidth !== sidebarWidth ? draft.sidebarWidth : null;
  useEffect(() => {
    sidebarWidthPreviewStore.set(widthPreview);
  }, [widthPreview]);
  useEffect(
    () => () => {
      tokenPreviewStore.set(null);
      sidebarWidthPreviewStore.set(null);
    },
    [],
  );

  if (!draft) return null;
  return (
    <SettingsForm
      title={t("sections.appearance")}
      description={t("form.browserLocal")}
      dirty={dirty}
      onDiscard={reset}
      onSave={async () => {
        if (Object.keys(preview).length) setTokens(preview);
        if (draft.stacksMode !== saved.stacksMode || draft.containersMode !== saved.containersMode) preferencesStore.set((p) => ({ ...p, stacksMode: draft.stacksMode, containersMode: draft.containersMode }));
        if (draft.sidebarWidth !== saved.sidebarWidth) setVisualStyle({ sidebarWidth: draft.sidebarWidth });
        commit(draft);
      }}
      note={ta("labHint")}
    >
      <FieldGroup>
        <p className="rounded-lg border border-primary/25 bg-primary/8 p-2.5 text-xs text-muted-foreground" aria-live="polite">
          {dirty ? ta("previewActive") : ta("previewHint")}
        </p>
        <Field>
          <HelpLabel topic="settingsAppearance.theme">{ta("theme")}</HelpLabel>
          <SegmentedToggle
            value={draft.theme}
            onChange={(theme) => patch({ theme })}
            label={ta("theme")}
            options={[
              { value: "dark", label: ta("dark"), icon: <Moon /> },
              { value: "light", label: ta("light"), icon: <Sun /> },
            ]}
            className="self-start"
          />
        </Field>
        <FieldSet>
          <HelpLegend topic="settingsAppearance.palette">{ta("palette")}</HelpLegend>
          <FieldDescription>{ta("paletteHint")}</FieldDescription>
          <PaletteEditor value={draft.palette} onChange={(palette) => patch({ palette })} theme={draft.theme} idPrefix="ap" />
        </FieldSet>
        <Field>
          <HelpLabel topic="settingsAppearance.density">{ta("density")}</HelpLabel>
          <SegmentedToggle value={draft.density} onChange={(density) => patch({ density })} label={ta("density")} options={(["compact", "comfortable", "spacious"] as const).map((d) => ({ value: d, label: ta(`densities.${d}`) }))} className="self-start" />
        </Field>
        <Field>
          <HelpLabel topic="settingsAppearance.motion">{ta("motion")}</HelpLabel>
          <SegmentedToggle value={draft.motion} onChange={(motion) => patch({ motion })} label={ta("motion")} options={(["off", "calm", "normal", "playful"] as const).map((m) => ({ value: m, label: ta(`motions.${m}`) }))} className="self-start" />
          <FieldDescription>{ta("motionHint")}</FieldDescription>
        </Field>
        <Field>
          <div className="flex items-center justify-between gap-3">
            <FieldLabel htmlFor="ap-sidebar-width">{ta("sidebarWidth")}</FieldLabel>
            <span className="font-mono text-xs text-muted-foreground tabular">{ta("rem", { value: draft.sidebarWidth })}</span>
          </div>
          <Slider
            id="ap-sidebar-width"
            value={[draft.sidebarWidth]}
            min={VISUAL_RANGES.sidebarWidth.min}
            max={VISUAL_RANGES.sidebarWidth.max}
            step={VISUAL_RANGES.sidebarWidth.step}
            onValueChange={([v]) => v !== undefined && patch({ sidebarWidth: v })}
            aria-label={ta("sidebarWidth")}
            aria-valuetext={ta("rem", { value: draft.sidebarWidth })}
          />
          <FieldDescription>{ta("sidebarWidthHint")}</FieldDescription>
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          {(["stacksMode", "containersMode"] as const).map((key) => (
            <Field key={key}>
              <HelpLabel topic="settingsAppearance.overviewModes">{ta(key)}</HelpLabel>
              <SegmentedToggle value={draft[key]} onChange={(v) => patch({ [key]: v })} label={ta(key)} options={(["compact", "advanced"] as const).map((m) => ({ value: m, label: ta(`modes.${m}`) }))} className="self-start" />
            </Field>
          ))}
        </div>
      </FieldGroup>
    </SettingsForm>
  );
}
