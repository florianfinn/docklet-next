// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original truthful component metadata panel without synthetic inspect data. M14 host maintenance and availability.

import { useTranslations } from "use-intl";
import type { SystemComponent } from "@/domain/system";
import { AdapterOutcomeError } from "@/data/capability";
import { useHostError } from "./use-host-error";
export function SystemMetadata({ component }: { component: SystemComponent }) {
  const t = useTranslations("hosts.components"), unavailable = useTranslations("hosts.card");
  const error = useHostError();
  return <section aria-label={t("title")} className="flex flex-col gap-2">
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm">
      <dt>{t("version")}</dt><dd className="break-all font-mono">{component.agentVersion || unavailable("unavailable")}</dd>
      <dt>{t("contract")}</dt><dd>{component.contractVersion ?? unavailable("unavailable")}</dd>
      <dt>{t("tools")}</dt><dd className="break-all font-mono">{component.toolsImage || unavailable("unavailable")}</dd>
    </dl>
    {component.updateRequired && <p role="status">{t("updateRequired")}</p>}
    {component.errorCode && <p role="status" className="text-sm text-muted-foreground">{error(new AdapterOutcomeError({ kind: "error", code: component.errorCode }))}</p>}
  </section>;
}
