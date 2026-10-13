import { Play, RotateCcw } from "lucide-react";
import { useTranslations } from "use-intl";

import { BrandLogo } from "@/shared/brand/BrandLogo";
import { replayBrandEntrance } from "@/shared/brand/brand-entrance";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { useUiStyle } from "@/shared/preferences/ui-style-store";
import { DEFAULT_VISUAL_STYLE, LOGO_MOTIONS, LOGO_VARIANTS, setVisualStyle, useVisualStyle, VISUAL_RANGES, type VisualStyle } from "@/shared/preferences/visual-style-store";
import { Button } from "@/shared/ui/button";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";
import { DotField } from "@/shared/visual/DotField";

import { useMotionEnabled } from "@/shared/theme/use-design-tokens";

type RangeKey = keyof typeof VISUAL_RANGES;

function RangeField({ field, label, unit, disabled }: { field: RangeKey; label: string; unit: "px" | "rem" | "seconds" | "percent"; disabled?: boolean }) {
  const t = useTranslations("visual.lab");
  const value = useVisualStyle(field);
  const { min, max, step } = VISUAL_RANGES[field];
  const id = `lab-visual-${field}`;
  return (
    <Field data-disabled={disabled || undefined}>
      <div className="flex items-center justify-between">
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        <span className="font-mono text-xs text-muted-foreground tabular">{t(unit, { value: unit === "percent" ? Math.round(value * 100) : value })}</span>
      </div>
      <Slider
        id={id}
        disabled={disabled}
        value={[value]}
        min={min}
        max={max}
        step={step}
        onValueChange={([v]) => setVisualStyle({ [field]: v ?? value } as Partial<VisualStyle>)}
        aria-label={label}
        aria-valuetext={t(unit, { value: unit === "percent" ? Math.round(value * 100) : value })}
      />
    </Field>
  );
}

/**
 * Logo, sidebar width, edge light reach and the dotted background wave. Rendered at the end
 * of the lab's element controls; every value applies live and stays local.
 */
export function VisualLabSection() {
  const t = useTranslations("visual.lab");
  const tb = useTranslations("visual.brand");
  const variant = useVisualStyle("logoVariant");
  const motionStyle = useVisualStyle("logoMotion");
  const entrance = useVisualStyle("logoEntrance");
  const wave = useVisualStyle("dotWave");
  const glow = useUiStyle("pointerGlow");
  const motion = useMotionEnabled();
  const resetGlow = () => setVisualStyle({ glowInside: DEFAULT_VISUAL_STYLE.glowInside, glowOutside: DEFAULT_VISUAL_STYLE.glowOutside, glowSize: DEFAULT_VISUAL_STYLE.glowSize, glowLine: DEFAULT_VISUAL_STYLE.glowLine });
  return (
    <FieldGroup data-slot="visual-lab">
      <FieldSet>
        <FieldLegend variant="label">{t("logo")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("logoHint")}</p>
        <div className="grid place-items-center rounded-xl border bg-background/50 p-4" data-slot="logo-preview">
          <BrandLogo key={`${variant}-${motionStyle}`} variant={variant} motionStyle={motionStyle} size={44} entranceSlot={`lab-${variant}-${motionStyle}`} motion={motion} label={tb("label")} />
        </div>
        <Field>
          <FieldLabel>{t("variant")}</FieldLabel>
          <SegmentedToggle label={t("variant")} value={variant} onChange={(logoVariant) => setVisualStyle({ logoVariant })} options={LOGO_VARIANTS.map((v) => ({ value: v, label: t(`variants.${v}`) }))} fit="content" wrap />
        </Field>
        <Field>
          <FieldLabel>{t("motion")}</FieldLabel>
          <SegmentedToggle label={t("motion")} value={motionStyle} onChange={(logoMotion) => setVisualStyle({ logoMotion })} options={LOGO_MOTIONS.map((m) => ({ value: m, label: t(`motions.${m}`) }))} fit="content" wrap />
        </Field>
        <Field orientation="horizontal">
          <Switch id="lab-logo-entrance" checked={entrance} onCheckedChange={(logoEntrance) => setVisualStyle({ logoEntrance })} />
          <FieldContent>
            <FieldLabel htmlFor="lab-logo-entrance">{t("entrance")}</FieldLabel>
            <FieldDescription>{t("entranceHint")}</FieldDescription>
          </FieldContent>
        </Field>
        <Button variant="outline" size="sm" className="self-start" disabled={!motion} onClick={replayBrandEntrance}>
          <Play data-icon="inline-start" />
          {t("replay")}
        </Button>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("sidebar")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("sidebarHint")}</p>
        <RangeField field="sidebarWidth" label={t("sidebarWidth")} unit="rem" />
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("glow")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("glowHint")}</p>
        <RangeField field="glowInside" label={t("glowInside")} unit="px" disabled={!glow} />
        <RangeField field="glowOutside" label={t("glowOutside")} unit="px" disabled={!glow} />
        <RangeField field="glowSize" label={t("glowSize")} unit="px" disabled={!glow} />
        <RangeField field="glowLine" label={t("glowLine")} unit="px" disabled={!glow} />
        <Button variant="ghost" size="sm" className="self-start" onClick={resetGlow}>
          <RotateCcw data-icon="inline-start" />
          {t("reset")}
        </Button>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("wave")}</FieldLegend>
        <Field orientation="horizontal">
          <Switch id="lab-dot-wave" checked={wave} onCheckedChange={(dotWave) => setVisualStyle({ dotWave })} />
          <FieldContent>
            <FieldLabel htmlFor="lab-dot-wave">{t("waveSwitch")}</FieldLabel>
            <FieldDescription>{t("waveHint")}</FieldDescription>
          </FieldContent>
        </Field>
        <RangeField field="dotWaveStrength" label={t("waveStrength")} unit="percent" disabled={!wave} />
        <RangeField field="dotWavePeriod" label={t("wavePeriod")} unit="seconds" disabled={!wave} />
        <RangeField field="dotDepth" label={t("depth")} unit="percent" disabled={!wave} />
        <p className="-mt-2 text-xs text-muted-foreground">{t("depthHint")}</p>
        <div className="dot-grid relative h-24 overflow-hidden rounded-xl border bg-card" data-slot="wave-preview" aria-hidden>
          <DotField />
        </div>
      </FieldSet>
    </FieldGroup>
  );
}
