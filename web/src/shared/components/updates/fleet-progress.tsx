// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import { useFleetProgress } from "./use-fleet-progress";
import { useTranslations } from "use-intl";
import { useHosts } from "@/data/queries";
import { FLEET_CODE_KEYS, FLEET_PHASE_KEYS, FLEET_OUTCOME_KEYS, FLEET_STATE_KEYS, FLEET_TARGET_OUTCOME_KEYS } from "@/data/wire-catalogue-self-update";
import { wireKey } from "@/data/wire-catalogue";
import type { FleetStatus, FleetTarget } from "@/domain/fleet-update";
import type { Host } from "@/domain/model";
import { openActionDialog } from "@/features/activity/action-dialog-store";
import { Button } from "@/shared/ui/button";

export function FleetTargetRow({ target, hosts = [] }: { target: FleetTarget; hosts?: Host[] }) {
  const all = useTranslations(), t = useTranslations("settings.updates.selfUpdate");
  const code = wireKey(FLEET_CODE_KEYS, target.code);
  return <div className="rounded-lg border p-3 text-sm" data-fleet-target={target.environmentId}>
    <p>{hosts.find(h => h.id === target.environmentId)?.name ?? target.environmentId} · {all(FLEET_STATE_KEYS[target.state])}{target.outcome && <> · {all(FLEET_TARGET_OUTCOME_KEYS[target.outcome])}</>}</p>
    {code && <p>{all(code)}</p>}
    {target.runId && <Button type="button" variant="link" onClick={() => openActionDialog(target.runId!)}>{t("viewRun")}</Button>}
  </div>;
}
export function FleetProgressCard({ status, hosts = [], dismiss }: { status: FleetStatus; hosts?: Host[]; dismiss?: () => void }) {
  const t = useTranslations("settings.updates.fleet"), all = useTranslations();
  return <section className="flex flex-col gap-2 rounded-xl border p-4" aria-label={t("title")}>
    <h4>{t("progress", { version: status.targetVersion ?? "" })}</h4>
    {status.phase && <p role="status">{all(FLEET_PHASE_KEYS[status.phase])}</p>}
    {status.outcome && <p>{all(FLEET_OUTCOME_KEYS[status.outcome])}</p>}
    {status.hub && <FleetTargetRow target={status.hub} hosts={hosts} />}
    {status.agents?.map(target => <FleetTargetRow key={target.environmentId} target={target} hosts={hosts} />)}
    {!status.active && dismiss && <Button type="button" variant="outline" onClick={dismiss}>{t("dismiss")}</Button>}
  </section>;
}
export function FleetRunOutcomes({ runId }: { runId: string }) {
  const query = useFleetProgress(), hosts = useHosts();
  return query.data?.runId === runId ? <FleetProgressCard status={query.data} hosts={hosts.data} /> : null;
}
