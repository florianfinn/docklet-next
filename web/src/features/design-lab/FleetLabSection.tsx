import { useTranslations } from "use-intl";

import { RING_SIZE, setFleetView, useFleetView, type CardsPlacement, type FleetMode } from "@/features/dashboard/fleet-view-store";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Field, FieldContent, FieldDescription, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

/** Experimental dashboard summary: fleet visualisation, ring size, host card placement and hover fan. */
export function FleetLabSection() {
  const t = useTranslations("lab.fleet");
  const view = useFleetView();
  return (
    <FieldSet>
      <FieldLegend variant="label">{t("title")}</FieldLegend>
      <p className="-mt-2 text-xs text-muted-foreground">{t("hint")}</p>
      <Field>
        <FieldLabel>{t("mode")}</FieldLabel>
        <SegmentedToggle<FleetMode>
          value={view.mode}
          onChange={(mode) => setFleetView({ mode })}
          label={t("mode")}
          options={(["joint", "perHost", "line"] as const).map((m) => ({ value: m, label: t(`modes.${m}`) }))}
          fit="content"
          wrap
          className="self-start"
        />
        <FieldDescription>{t(`modeHints.${view.mode}`)}</FieldDescription>
      </Field>
      <Field>
        <div className="flex items-center justify-between">
          <FieldLabel htmlFor="lab-ring-size">{t("ringSize")}</FieldLabel>
          <span className="font-mono text-xs text-muted-foreground tabular">{view.ringSize} px</span>
        </div>
        <Slider id="lab-ring-size" disabled={view.mode === "line"} value={[view.ringSize]} min={RING_SIZE.min} max={RING_SIZE.max} step={10} onValueChange={([v]) => setFleetView({ ringSize: v ?? view.ringSize })} aria-label={t("ringSize")} />
      </Field>
      <Field>
        <FieldLabel>{t("placement")}</FieldLabel>
        <SegmentedToggle<CardsPlacement>
          value={view.placement}
          onChange={(placement) => setFleetView({ placement })}
          label={t("placement")}
          options={(["below", "right"] as const).map((p) => ({ value: p, label: t(`placements.${p}`) }))}
          className="self-start"
        />
        <FieldDescription>{t("placementHint")}</FieldDescription>
      </Field>
      <Field orientation="horizontal">
        <Switch id="lab-ring-fan" checked={view.fan} onCheckedChange={(fan) => setFleetView({ fan })} />
        <FieldContent>
          <FieldLabel htmlFor="lab-ring-fan">{t("fan")}</FieldLabel>
          <FieldDescription>{t("fanHint")}</FieldDescription>
        </FieldContent>
      </Field>
    </FieldSet>
  );
}
