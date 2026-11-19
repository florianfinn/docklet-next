// Source: docklet-next/issues/522
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original setup completion: one logo entrance, then the dashboard (replaces the Done step).

import { useEffect, useRef } from "react";
import { useTranslations } from "use-intl";

import { BrandLogo } from "@/shared/brand/BrandLogo";
import { useMotionEnabled } from "@/shared/theme/use-design-tokens";
import { DotField } from "@/shared/visual/DotField";

/** Length of one logo entrance; reduced motion continues at once. */
export const COMPLETION_MS = 1400;

export function SetupCompletion({ onDone }: { onDone: () => void }) {
  const t = useTranslations("setup.completion");
  const motion = useMotionEnabled();
  const done = useRef(onDone);
  useEffect(() => { done.current = onDone; }, [onDone]);
  useEffect(() => {
    const timer = setTimeout(() => done.current(), motion ? COMPLETION_MS : 0);
    return () => clearTimeout(timer);
  }, [motion]);
  return (
    <main className="dot-grid grid min-h-svh place-items-center p-4" aria-busy="true" data-slot="setup-completion">
      <DotField fixed />
      <div className="flex flex-col items-center gap-5 text-center">
        <BrandLogo variant="stacked" size={112} entranceSlot="setup-completion" motion={motion} />
        <p role="status" className="text-sm text-muted-foreground">{t("preparing")}</p>
      </div>
    </main>
  );
}
