import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { isRunBusy } from "@/data/run-gate";
import { useAdapter } from "@/data/use-adapter";
import { withPercent } from "@/domain/locale";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { useAdapterText } from "@/data/use-adapter-text";
import { queueDemoFailure } from "@/runtime";
import { cn } from "@/shared/lib/utils";
import { ACTION_DIALOG_DESIGNS, ACTION_PRESENTATION_OVERRIDES, RUN_MOTIONS, setUiStyle, TOAST_DESIGNS, TOAST_SCALE, toastDetail, useUiStyle, type ActionPresentationOverride } from "@/shared/preferences/ui-style-store";
import { Button } from "@/shared/ui/button";
import { Field, FieldContent, FieldDescription, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Slider } from "@/shared/ui/slider";

import { StepTrack } from "./ActionSteps";
import { ActionToastView } from "./ActionToastView";
import { sampleRun } from "./sample-run";

function Choice({ active, onSelect, title, description, children }: { active: boolean; onSelect: () => void; title: string; description: string; children?: React.ReactNode }) {
  return (
    <div className={cn("flex flex-col gap-2 rounded-xl border bg-card/60 p-3", active && "border-primary/60 bg-primary/10")}>
      <button type="button" role="radio" aria-checked={active} onClick={onSelect} className="flex flex-col gap-1 rounded-md text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-xs text-muted-foreground">{description}</span>
      </button>
      {children && (
        // Static preview; selection happens with the radio above.
        <div inert aria-hidden className="pointer-events-none [--width:100%]">
          {children}
        </div>
      )}
    </div>
  );
}

/** Design-lab controls for notices, action toasts and the action dialog, with live samples. */
export function NotificationLabSection() {
  const t = useTranslations("lab.notifications");
  const dt = useAdapterText();
  const adapter = useAdapter();
  const design = useUiStyle("toastDesign");
  const dialogDesign = useUiStyle("actionDialogDesign");
  const presentation = useUiStyle("actionPresentationOverride");
  const scale = useUiStyle("toastScale");
  const runMotion = useUiStyle("runMotion");
  // Frozen preview: seven seconds into the sample run.
  const [sample] = useState(() => {
    const now = Date.now();
    return { run: sampleRun(new Date(now - 7000).toISOString()), now };
  });
  const run = (start: () => Promise<unknown>) => void start().catch((e: unknown) => isRunBusy(e) || toast.error(dt(e instanceof Error ? e.message : String(e))));

  return (
    <>
      <FieldSet>
        <FieldLegend variant="label">{t("toastDesign")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("toastDesignHint")}</p>
        <div role="radiogroup" aria-label={t("toastDesign")} className="flex flex-col gap-2">
          {TOAST_DESIGNS.map((d) => (
            <Choice key={d} active={design === d} onSelect={() => setUiStyle({ toastDesign: d })} title={t(`toastDesigns.${d}`)} description={t(`toastDesignHints.${d}`)}>
              <ActionToastView run={sample.run} design={d} detail={toastDetail(scale)} now={sample.now} major onClose={() => {}} onOpen={() => {}} />
            </Choice>
          ))}
        </div>
        <Field>
          <div className="flex items-center justify-between">
            <FieldLabel htmlFor="lab-toast-size">{t("toastSize")}</FieldLabel>
            <span className="font-mono text-xs text-muted-foreground tabular">
              {withPercent(Math.round(scale * 100))} · {t(`toastDetail.${toastDetail(scale)}`)}
            </span>
          </div>
          <Slider id="lab-toast-size" value={[scale]} min={TOAST_SCALE.min} max={TOAST_SCALE.max} step={0.05} onValueChange={([v]) => setUiStyle({ toastScale: v ?? scale })} aria-label={t("toastSize")} />
          <FieldDescription>{t("toastSizeHint")}</FieldDescription>
        </Field>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("runMotion")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("runMotionHint")}</p>
        <div role="radiogroup" aria-label={t("runMotion")} className="flex flex-col gap-2">
          {RUN_MOTIONS.map((m) => (
            <Choice key={m} active={runMotion === m} onSelect={() => setUiStyle({ runMotion: m })} title={t(`runMotions.${m}`)} description={t(`runMotionHints.${m}`)}>
              <StepTrack steps={sample.run.steps} status="running" labels motion={m} />
            </Choice>
          ))}
        </div>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("dialogDesign")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("dialogDesignHint")}</p>
        <div role="radiogroup" aria-label={t("dialogDesign")} className="flex flex-col gap-2">
          {ACTION_DIALOG_DESIGNS.map((d) => (
            <Choice key={d} active={dialogDesign === d} onSelect={() => setUiStyle({ actionDialogDesign: d })} title={t(`dialogDesigns.${d}`)} description={t(`dialogDesignHints.${d}`)} />
          ))}
        </div>
        <Field>
          <FieldContent>
            <FieldLabel>{t("dialogAuto")}</FieldLabel>
            <FieldDescription>{t("dialogAutoHint")}</FieldDescription>
          </FieldContent>
          <SegmentedToggle<ActionPresentationOverride>
            value={presentation}
            onChange={(actionPresentationOverride) => setUiStyle({ actionPresentationOverride })}
            label={t("dialogAuto")}
            options={ACTION_PRESENTATION_OVERRIDES.map((value) => ({ value, label: t(`dialogAutoModes.${value}`) }))}
            className="self-start"
          />
        </Field>
      </FieldSet>
      <FieldSet>
        <FieldLegend variant="label">{t("samples")}</FieldLegend>
        <p className="-mt-2 text-xs text-muted-foreground">{t("samplesHint")}</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => run(() => adapter.runStackCommand("cloud", "redeploy"))}>
            {t("sampleMajor")}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={() => run(() => { queueDemoFailure(adapter, { operation: "runStackCommand", at: "pull" }); return adapter.runStackCommand("home", "redeploy"); })}>
            {t("sampleMajorFail")}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={() => run(() => Promise.all([adapter.runStackCommand("media", "redeploy"), adapter.runStackCommand("craft", "redeploy")]))}>
            {t("sampleParallel")}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={() => run(() => adapter.checkUpdates({ stackId: "cloud" }))}>
            {t("sampleMinor")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              toast.success(t("noticeSaved"));
              setTimeout(() => toast.info(t("noticeInfo")), 350);
              setTimeout(() => toast.warning(t("noticeWarn")), 700);
              setTimeout(() => toast.error(t("noticeError"), { description: t("noticeErrorDetail") }), 1050);
            }}
          >
            {t("sampleNotices")}
          </Button>
        </div>
      </FieldSet>
    </>
  );
}
