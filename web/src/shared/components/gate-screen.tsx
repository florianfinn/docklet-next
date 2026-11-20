// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original shared gate surface and non-overlapping hub reachability retries.

import type { ReactNode } from "react";
import { useTranslations } from "use-intl";
import { BrandLogo } from "@/shared/brand/BrandLogo";
import { DotField } from "@/shared/visual/DotField";
import { Card } from "@/shared/ui/card";
import { Button } from "@/shared/ui/button";

/** A page title renders as the heading; a message is a status (busy) or an alert (error). */
export function GateScreen({ title, message, action, busy = false, retrying = false, children, slot = "gate-screen" }: { title?: string; message?: string; action?: ReactNode; busy?: boolean; retrying?: boolean; children?: ReactNode; slot?: string }) {
  return <main className="dot-grid grid min-h-svh place-items-center p-4" aria-busy={busy || retrying} data-slot={slot}><DotField fixed /><Card className="w-full max-w-md gap-5 p-6"><BrandLogo size={36} motion={false} />{title && <h1 className="text-xl font-semibold">{title}</h1>}{message && <p role={busy ? "status" : "alert"}>{message}</p>}{action}{children}</Card></main>;
}
export function UnreachableGate({ retry, retrying }: { retry: () => Promise<unknown>; retrying: boolean }) {
  const t = useTranslations("login");
  return <GateScreen message={t("unreachable")} retrying={retrying} action={<Button disabled={retrying} onClick={() => void retry()}>{t("retry")}</Button>} />;
}
