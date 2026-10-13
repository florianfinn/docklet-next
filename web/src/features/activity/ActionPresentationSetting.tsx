import { ArrowRight, MonitorPlay } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import type { ActionPresentation } from "@/domain/settings";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { IconTile } from "@/shared/components/tone";
import { useUiStyle } from "@/shared/preferences/ui-style-store";
import { Card } from "@/shared/ui/card";
import { HelpLabel } from "@/shared/help/help-tooltip";
import { Field, FieldContent, FieldDescription } from "@/shared/ui/field";

/** Note shown wherever the hub value is edited or summarised while the design lab overrides it in this browser. */
function OverrideNote() {
  const t = useTranslations("activity.presentation");
  const override = useUiStyle("actionPresentationOverride");
  if (override === "hub") return null;
  return (
    <p className="text-xs text-warning" data-slot="presentation-override">
      {t("overridden", { mode: t(override) })}
    </p>
  );
}

/**
 * Hub setting for how major actions present themselves. It belongs to the
 * activity draft of the hub settings, so Save and Discard apply as for every
 * other field; the design-lab override is only named here.
 */
export function ActionPresentationField({ value, onChange }: { value: ActionPresentation; onChange: (value: ActionPresentation) => void }) {
  const t = useTranslations("activity.presentation");
  const design = useUiStyle("actionDialogDesign");
  return (
    <Field data-slot="action-presentation-setting">
      <FieldContent>
        <HelpLabel topic="settingsActivity.presentation">{t("title")}</HelpLabel>
        <FieldDescription>{t("hint")}</FieldDescription>
      </FieldContent>
      <SegmentedToggle<ActionPresentation>
        value={value}
        onChange={onChange}
        label={t("title")}
        options={[
          { value: "dialog", label: t("dialog") },
          { value: "toast", label: t("toast") },
        ]}
        className="self-start"
      />
      <FieldDescription>{value === "dialog" ? t("dialogHint", { design: t(`designs.${design}`) }) : t("toastHint")}</FieldDescription>
      <OverrideNote />
    </Field>
  );
}

/** Summary on the notifications page; the value itself is edited under Activity. */
export function ActionPresentationPointer({ value }: { value: ActionPresentation }) {
  const t = useTranslations("activity.presentation");
  return (
    <Card className="max-w-5xl flex-row flex-wrap items-center gap-3 p-4" data-slot="action-presentation-pointer">
      <IconTile size="sm">
        <MonitorPlay />
      </IconTile>
      <div className="min-w-0 flex-1">
        <h3 className="text-sm font-semibold">{t("title")}</h3>
        <p className="text-xs text-muted-foreground">{t("current", { mode: t(value) })}</p>
        <OverrideNote />
      </div>
      <Link to="/settings/activity" className="inline-flex items-center gap-1 text-sm font-medium text-primary-text underline-offset-4 hover:underline">
        {t("change")}
        <ArrowRight className="size-3.5" aria-hidden />
      </Link>
    </Card>
  );
}
