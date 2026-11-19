// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original shared first-run chrome, split from SetupPage for the identity claim step.

import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { DesignLabPanel } from "@/runtime";
import { BrandLogo } from "@/shared/brand/BrandLogo";
import { StepRail } from "@/shared/components/step-rail";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { useMotionEnabled, useTokens } from "@/shared/theme/use-design-tokens";
import { Button } from "@/shared/ui/button";
import { Toaster } from "@/shared/ui/sonner";
import { DotField } from "@/shared/visual/DotField";

import { READABLE_PLATE } from "./readable-plate";
import { SETUP_STEPS, type SetupStep } from "./setup-draft";

interface SetupFrameProps {
  connected: boolean;
  current: SetupStep;
  /** Highest reachable step index; -1 locks the rail. */
  reachable: number;
  onSelect: (step: SetupStep) => void;
  children: ReactNode;
}

/** Header, step rail and background shared by the identity claim and the configuration wizard. */
export function SetupFrame({ connected, current, reachable, onSelect, children }: SetupFrameProps) {
  const t = useTranslations("setup");
  const tokens = useTokens();
  const motion = useMotionEnabled();
  const steps = SETUP_STEPS.map((id) => ({ id, title: t(`steps.${id}.title`), description: t(`steps.${id}.description`) }));
  return (
    <div className="dot-grid min-h-svh">
      <DotField fixed />
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-6 md:px-8 md:py-10">
        <header className={cn(READABLE_PLATE, "flex flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3")}>
          <BrandLogo size={44} entranceSlot="setup" motion={motion} className="shrink-0" />
          <div className="mr-auto">
            <ToneBadge tone="violet" className="mb-1">
              {t(connected ? "badgeConnected" : "badge")}
            </ToneBadge>
            <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
          </div>
          <LanguageSwitch size="xs" />
          {!connected && <Button asChild variant="outline" size="sm">
            <Link to="/dashboard">
              <ArrowLeft data-icon="inline-start" />
              {t("exit")}
            </Link>
          </Button>}
        </header>
        <p className={cn(READABLE_PLATE, "max-w-3xl self-start px-4 py-2.5 text-sm text-muted-foreground")}>{t(connected ? "leadConnected" : "lead")}</p>
        <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
          <div className={cn(READABLE_PLATE, "p-1.5")} data-slot="setup-rail">
            <StepRail steps={steps} current={current} reachable={reachable} onSelect={(id) => onSelect(id as SetupStep)} label={t("railLabel")} />
          </div>
          {children}
        </div>
      </div>
      <Toaster position="top-right" offset={{ top: 24, right: 16 }} theme={tokens.theme} />
      <DesignLabPanel />
    </div>
  );
}
