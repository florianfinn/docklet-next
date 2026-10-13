import { withPercent } from "@/domain/locale";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Moon, RotateCw, Sun } from "lucide-react";
import { useTranslations } from "use-intl";

import type { FailAt } from "@/data/demo/options";
import { simulationStore, type FailArea, type Latency } from "@/data/demo/simulation";
import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";
import { PaletteEditor } from "@/shared/theme/PaletteEditor";
import { Button } from "@/shared/ui/button";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

import { setTokens, TOKEN_RANGES, tokenPreviewStore, type Density, type MotionLevel } from "@/shared/theme/design-tokens";
import { usePrefersReducedMotion, useTokens } from "@/shared/theme/use-design-tokens";

function SliderField({ id, label, hint, value, min, max, step, display, onChange }: { id: string; label: string; hint?: string; value: number; min: number; max: number; step: number; display: string; onChange: (v: number) => void }) {
  return (
    <Field>
      <div className="flex items-center justify-between">
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        <span className="font-mono text-xs text-muted-foreground tabular">{display}</span>
      </div>
      <Slider id={id} value={[value]} min={min} max={max} step={step} onValueChange={([v]) => onChange(v ?? value)} aria-label={label} />
      {hint && <FieldDescription>{hint}</FieldDescription>}
    </Field>
  );
}

export function LookSection() {
  const t = useTranslations("lab");
  const tokens = useTokens();
  const preview = useStore(tokenPreviewStore, (p) => p);
  return (
    <FieldGroup>
      <FieldSet>
        <FieldLegend variant="label">{t("theme.label")}</FieldLegend>
        <SegmentedToggle
          value={tokens.theme}
          onChange={(theme) => setTokens({ theme })}
          label={t("theme.label")}
          options={[
            { value: "dark", label: t("theme.dark"), icon: <Moon /> },
            { value: "light", label: t("theme.light"), icon: <Sun /> },
          ]}
          className="self-start"
        />
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("palette.label")}</FieldLegend>
        <FieldDescription>{t("palette.hint")}</FieldDescription>
        {preview?.palette && <p className="rounded-lg border border-warning/40 bg-warning/10 p-2.5 text-xs text-warning">{t("palette.previewActive")}</p>}
        <PaletteEditor value={tokens.palette} onChange={(palette) => setTokens({ palette })} theme={preview?.theme ?? tokens.theme} idPrefix="lab" />
      </FieldSet>
      <SliderField id="lab-sem" label={t("semantic.label")} hint={t("semantic.hint")} value={tokens.semantic} {...TOKEN_RANGES.semantic} display={withPercent(Math.round(tokens.semantic * 100))} onChange={(v) => setTokens({ semantic: v })} />
      <SliderField id="lab-glow" label={t("glow.label")} hint={t("glow.hint")} value={tokens.glow} {...TOKEN_RANGES.glow} display={withPercent(Math.round(tokens.glow * 100))} onChange={(v) => setTokens({ glow: v })} />
    </FieldGroup>
  );
}

export function MotionSection() {
  const t = useTranslations("lab.motion");
  const tokens = useTokens();
  const reduced = usePrefersReducedMotion();
  return (
    <FieldGroup>
      <FieldSet>
        <FieldLegend variant="label">{t("label")}</FieldLegend>
        <SegmentedToggle<MotionLevel> value={tokens.motion} onChange={(motion) => setTokens({ motion })} label={t("label")} options={(["off", "calm", "normal", "playful"] as const).map((m) => ({ value: m, label: t(m) }))} fit="content" wrap className="self-start" />
      </FieldSet>
      <SliderField id="lab-speed" label={t("speed")} value={tokens.speed} {...TOKEN_RANGES.speed} display={t("speedValue", { value: tokens.speed.toFixed(1) })} onChange={(v) => setTokens({ speed: v })} />
      <p className={cn("rounded-lg border p-3 text-xs", reduced ? "border-warning/40 bg-warning/10 text-warning" : "text-muted-foreground")}>{reduced ? t("reduced") : t("reducedNone")}</p>
      {reduced && (
        <Field orientation="horizontal">
          <Switch id="lab-ignore" checked={tokens.ignoreReducedMotion} onCheckedChange={(v) => setTokens({ ignoreReducedMotion: v })} />
          <FieldLabel htmlFor="lab-ignore">{t("ignoreReduced")}</FieldLabel>
        </Field>
      )}
      <p className="text-xs text-muted-foreground">{t("noPageMotion")}</p>
    </FieldGroup>
  );
}

export function LayoutSection() {
  const t = useTranslations("lab.layout");
  const tokens = useTokens();
  return (
    <FieldGroup>
      <FieldSet>
        <FieldLegend variant="label">{t("density")}</FieldLegend>
        <SegmentedToggle<Density> value={tokens.density} onChange={(density) => setTokens({ density })} label={t("density")} options={(["compact", "comfortable", "spacious"] as const).map((d) => ({ value: d, label: t(d) }))} fit="content" wrap className="self-start" />
      </FieldSet>
      <SliderField id="lab-radius" label={t("radius")} value={tokens.radius} {...TOKEN_RANGES.radius} display={t("radiusValue", { value: tokens.radius.toFixed(2) })} onChange={(v) => setTokens({ radius: v })} />
      <SliderField id="lab-font" label={t("fontScale")} value={tokens.fontScale} {...TOKEN_RANGES.fontScale} display={t("fontValue", { value: Math.round(tokens.fontScale * 100) })} onChange={(v) => setTokens({ fontScale: v })} />
    </FieldGroup>
  );
}

function SimSelect<T extends string>({ id, label, value, options, onChange }: { id: string; label: string; value: T; options: { value: T; label: string }[]; onChange: (v: T) => void }) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select value={value} onValueChange={(v) => onChange(v as T)}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  );
}

export function SimulationSection({ onReload }: { onReload: () => void }) {
  const t = useTranslations("lab.simulation");
  const sim = useStore(simulationStore, (s) => s);
  const set = (patch: Partial<typeof sim>) => simulationStore.set((s) => ({ ...s, ...patch }));
  return (
    <FieldGroup>
      <SimSelect<Latency>
        id="sim-latency"
        label={t("latency")}
        value={sim.latency}
        onChange={(latency) => set({ latency })}
        options={(["instant", "short", "long", "endless"] as const).map((v) => ({ value: v, label: t(v) }))}
      />
      <SimSelect<FailArea>
        id="sim-fail"
        label={t("failArea")}
        value={sim.failArea}
        onChange={(failArea) => set({ failArea })}
        options={[
          { value: "none", label: t("failNone") },
          { value: "all", label: t("failAll") },
          { value: "dashboard", label: t("failDashboard") },
          { value: "stacks", label: t("failStacks") },
          { value: "containers", label: t("failContainers") },
          { value: "security", label: t("failSecurity") },
          { value: "networks", label: t("failNetworks") },
          { value: "volumes", label: t("failVolumes") },
          { value: "images", label: t("failImages") },
          { value: "hosts", label: t("failHosts") },
          { value: "settings", label: t("failSettings") },
        ]}
      />
      <Field orientation="horizontal">
        <Switch id="sim-empty" checked={sim.empty} onCheckedChange={(empty) => set({ empty })} />
        <FieldContent>
          <FieldLabel htmlFor="sim-empty">{t("empty")}</FieldLabel>
          <FieldDescription>{t("emptyHint")}</FieldDescription>
        </FieldContent>
      </Field>
      <Field orientation="horizontal">
        <Switch id="sim-live" checked={sim.live} onCheckedChange={(live) => set({ live })} />
        <FieldContent>
          <FieldLabel htmlFor="sim-live">{t("live")}</FieldLabel>
          <FieldDescription>{t("liveHint")}</FieldDescription>
        </FieldContent>
      </Field>
      <SliderField id="sim-speed" label={t("actionSpeed")} value={sim.actionSpeed} min={0.3} max={3} step={0.1} display={`${sim.actionSpeed.toFixed(1)}×`} onChange={(actionSpeed) => set({ actionSpeed })} />
      <SimSelect<FailAt>
        id="sim-nextfail"
        label={t("nextFail")}
        value={sim.nextActionFails}
        onChange={(nextActionFails) => set({ nextActionFails })}
        options={[
          { value: "none", label: t("nextFailNone") },
          { value: "pull", label: t("nextFailPull") },
          { value: "start", label: t("nextFailStart") },
          { value: "health", label: t("nextFailHealth") },
        ]}
      />
      <Button variant="outline" onClick={onReload} className="group self-start">
        <RotateCw data-icon="inline-start" className="spin-on-hover" />
        {t("reload")}
      </Button>
    </FieldGroup>
  );
}
