// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 3d2d4f27b524b3422d010d6a69a22fe57225a78f
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original production boundary and regression coverage for issue #27.

import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import type { ResourceRef } from "@/data/capability";
import { useCapability } from "@/data/use-capability";

/** Keep unsupported flows unmounted, including their reads and action handlers. */
export function CapabilityGate({ operation, resourceRef, children }: { operation: string; resourceRef?: ResourceRef; children: ReactNode }) {
  const capability = useCapability(operation, resourceRef);
  const t = useTranslations("common.query");
  return capability.available ? children : <div role="status" className="space-y-2 text-sm text-muted-foreground"><p>{t("unavailableTitle")}</p><p>{capability.reason}</p></div>;
}
