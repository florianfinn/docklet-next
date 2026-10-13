import { LogIn, Rows3, Rows4, Rocket, Server } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { withPercent } from "@/domain/locale";
import { onboardingStore } from "@/features/hosts/onboarding/onboarding-store";
import { restartSetup } from "@/features/setup/setup-draft";
import { LoadView } from "@/shared/components/container-load";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { cn } from "@/shared/lib/utils";
import { NotificationLabSection } from "@/features/activity/NotificationLab";
import { setUiStyle, useUiStyle, type HostFilterStyle, type MetricDisplay, type ToggleStyle } from "@/shared/preferences/ui-style-store";
import { Button } from "@/shared/ui/button";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

import { FleetLabSection } from "./FleetLabSection";
import { VisualLabSection } from "./VisualLabSection";
import { labOpenStore } from "@/features/design-lab/lab-open-store";

function OptionCard({ active, onSelect, title, description, children }: { active: boolean; onSelect: () => void; title: string; description: string; children?: React.ReactNode }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={active}
      onClick={onSelect}
      className={cn("lift flex w-full flex-col gap-2 rounded-xl border bg-card/60 p-3 text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50", active && "border-primary/60 bg-primary/10")}
    >
      <span className="block text-sm font-medium">{title}</span>
      <span className="block text-xs text-muted-foreground">{description}</span>
      {children}
    </button>
  );
}

/** Live comparison of toggle styles and metric display; the choice applies app-wide. */
export function ControlsSection() {
  const t = useTranslations("lab.controls");
  const tm = useTranslations("stacks.mode");
  const toggleStyle = useUiStyle("toggleStyle");
  const metricDisplay = useUiStyle("metricDisplay");
  const hostFilterStyle = useUiStyle("hostFilterStyle");
  const glow = useUiStyle("pointerGlow");
  const glowIntensity = useUiStyle("pointerGlowIntensity");
  const [mode, setMode] = useState<"compact" | "advanced">("compact");
  const [on, setOn] = useState(true);
  const [level, setLevel] = useState(60);
  return (
    <FieldGroup>
      <FieldSet>
        <FieldLegend variant="label">{t("toggleStyle")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("toggleStyleHint")}</p>
        <div role="radiogroup" aria-label={t("toggleStyle")} className="flex flex-col gap-2">
          {(["pill", "glow", "minimal"] as ToggleStyle[]).map((s) => (
            <OptionCard key={s} active={toggleStyle === s} onSelect={() => setUiStyle({ toggleStyle: s })} title={t(`styles.${s}`)} description={t(`styleHints.${s}`)} />
          ))}
        </div>
      </FieldSet>
      <section aria-label={t("preview")} className="flex flex-col gap-3 rounded-xl border bg-background/50 p-3">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{t("preview")}</span>
        <SegmentedToggle
          value={mode}
          onChange={setMode}
          label={t("previewToggle")}
          options={[
            { value: "compact", label: tm("compact"), icon: <Rows3 /> },
            { value: "advanced", label: tm("advanced"), icon: <Rows4 /> },
          ]}
          className="self-start"
        />
        <label className="flex items-center gap-2.5 text-sm">
          <Switch checked={on} onCheckedChange={setOn} aria-label={t("previewSwitch")} />
          {t("previewSwitch")}
        </label>
        <Slider value={[level]} min={0} max={100} step={1} onValueChange={([v]) => setLevel(v ?? level)} aria-label={t("previewSlider")} />
      </section>
      <FieldSet>
        <FieldLegend variant="label">{t("metrics")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("metricsHint")}</p>
        <div role="radiogroup" aria-label={t("metrics")} className="flex flex-col gap-2">
          {(["rings", "bars"] as MetricDisplay[]).map((d) => (
            <OptionCard key={d} active={metricDisplay === d} onSelect={() => setUiStyle({ metricDisplay: d })} title={t(`displays.${d}`)} description={t(`displayHints.${d}`)}>
              <span className="flex flex-col gap-2 rounded-lg border bg-background/40 p-2">
                <LoadView values={{ cpu: 12.4, memMb: 412 }} memLimitMb={1024} display={d} />
                <LoadView values={{ cpu: 93, memMb: 820 }} memLimitMb={1024} display={d} />
                <LoadView values={null} memLimitMb={1024} display={d} />
              </span>
            </OptionCard>
          ))}
        </div>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("hostFilter")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("hostFilterHint")}</p>
        <div role="radiogroup" aria-label={t("hostFilter")} className="flex flex-col gap-2">
          {(["list", "chips", "menu"] as HostFilterStyle[]).map((d) => (
            <OptionCard key={d} active={hostFilterStyle === d} onSelect={() => setUiStyle({ hostFilterStyle: d })} title={t(`hostFilters.${d}`)} description={t(`hostFilterHints.${d}`)} />
          ))}
        </div>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("glow")}</FieldLegend>
        <Field orientation="horizontal">
          <Switch id="lab-pointer-glow" checked={glow} onCheckedChange={(pointerGlow) => setUiStyle({ pointerGlow })} />
          <FieldContent>
            <FieldLabel htmlFor="lab-pointer-glow">{t("glowSwitch")}</FieldLabel>
            <FieldDescription>{t("glowHint")}</FieldDescription>
          </FieldContent>
        </Field>
        <Field>
          <div className="flex items-center justify-between">
            <FieldLabel htmlFor="lab-pointer-glow-intensity">{t("glowIntensity")}</FieldLabel>
            <span className="font-mono text-xs text-muted-foreground tabular">{withPercent(Math.round(glowIntensity * 100))}</span>
          </div>
          <Slider id="lab-pointer-glow-intensity" disabled={!glow} value={[glowIntensity]} min={0.2} max={1} step={0.05} onValueChange={([v]) => setUiStyle({ pointerGlowIntensity: v ?? glowIntensity })} aria-label={t("glowIntensity")} />
        </Field>
      </FieldSet>
      <NotificationLabSection />
      <FleetLabSection />
      <VisualLabSection />
    </FieldGroup>
  );
}

/** Replays the onboarding flows without touching the default dashboard. */
export function FlowsSection() {
  const t = useTranslations("lab.flows");
  const navigate = useNavigate();
  return (
    <FieldSet>
      <FieldLegend variant="label">{t("title")}</FieldLegend>
      <p className="-mt-2 text-xs text-muted-foreground">{t("hint")}</p>
      <LanguageSwitch size="xs" />
      <div className="flex flex-col gap-2">
        <Button
          variant="outline"
          className="justify-start"
          onClick={() => {
            restartSetup();
            labOpenStore.set(false);
            navigate("/setup?step=hub");
          }}
        >
          <Rocket data-icon="inline-start" />
          {t("setup")}
        </Button>
        <Button
          variant="outline"
          className="justify-start"
          onClick={() => {
            onboardingStore.set(null);
            labOpenStore.set(false);
            navigate("/hosts/new");
          }}
        >
          <Server data-icon="inline-start" />
          {t("host")}
        </Button>
        {(["settings", "local", "proxy"] as const).map((variant) => (
          <Button
            key={variant}
            variant="outline"
            className="justify-start"
            onClick={() => {
              labOpenStore.set(false);
              navigate(variant === "settings" ? "/login" : `/login?variant=${variant}`);
            }}
          >
            <LogIn data-icon="inline-start" />
            {variant === "settings" ? t("login") : t(variant === "local" ? "loginLocal" : "loginProxy")}
          </Button>
        ))}
        <p className="text-xs text-muted-foreground">{t("loginHint")}</p>
      </div>
    </FieldSet>
  );
}
