// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original role-scoped command palette using existing query consumers. M19 web consumer integration. Check all updates gated on the operation it runs.

import { useTranslations } from "use-intl";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { useCommandError } from "@/data/use-command-error";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useContainers, useHosts, useImages, useTopology, useVolumes } from "@/data/queries";
import { openActivity } from "@/features/activity/activity-store";
import { IDS } from "@/features/settings/settings-sections";
import type { Stack } from "@/domain/model";
import type { SearchEntry } from "./search-model";
import { SearchGroup } from "./SearchGroup";
import { NAV } from "./nav-items";
interface Props { query: string; select: (to?: string, run?: () => void) => void; hostName: (id: string) => string; pages: SearchEntry[]; actions: SearchEntry[]; stacks: Stack[]; stacksLoading: boolean }
/** Instantiated only for administrators; these hooks never mount in the reduced shell. */
export function AdminSearchGroups({ query, select, hostName, pages, actions: sharedActions, stacks, stacksLoading }: Props) {
  const hosts = useHosts(), containers = useContainers(), images = useImages(), volumes = useVolumes(), topology = useTopology();
  const t = useTranslations("shell.search"), nav = useTranslations("shell.nav"), settings = useTranslations("settings.sections"), adapter = useAdapter();
  const commandError = useCommandError();
  const create = useCapability("deployStack"), add = useCapability("registerHost"), updates = useCapability(adapter.checkAllUpdates ? "checkAllUpdates" : "checkUpdates"), activity = useCapability("listActions");
  const actions = [
    ...(create.available ? [{ id: "new-stack", label: t("newStack"), to: "/stacks/new" }] : []),
    ...(add.available ? [{ id: "add-host", label: nav("newHost"), to: "/hosts/new" }] : []),
    ...(updates.available ? [{ id: "updates", label: t("checkUpdates"), run: () => { const failure = captureCommandFailure(adapter); void (adapter.checkAllUpdates ? adapter.checkAllUpdates() : adapter.checkUpdates({})).catch((error) => failure.error(commandError(error))); } }] : []),
    ...(activity.available ? [{ id: "activity", label: t("activity"), run: () => openActivity() }] : []),
  ];
  return <><SearchGroup name={t("pages")} entries={[...pages, ...NAV.flatMap((group) => group.items).filter((item) => !item.preview && !["dashboard", "stacks"].includes(item.key)).map((item) => ({ id: item.key, label: nav(item.key), to: item.to }))]} query={query} select={select} /><SearchGroup name={t("settings")} entries={IDS.map((id) => ({ id, label: settings(id), keywords: [id], to: `/settings/${id}` }))} query={query} select={select} />
    <SearchGroup name={nav("hosts")} entries={hosts.data?.map((host) => ({ id: host.id, label: host.name, to: "/hosts" })) ?? []} query={query} select={select} loading={hosts.isPending} list="/hosts" />
    <SearchGroup name={nav("stacks")} entries={stacks.map((item) => ({ id: item.id, label: item.name, host: hostName(item.hostId), keywords: item.labels, to: `/stacks/${encodeURIComponent(item.id)}` }))} query={query} select={select} loading={stacksLoading} list="/stacks" />
    <SearchGroup name={nav("containers")} entries={containers.data?.map((item) => ({ id: item.id, label: item.name, keywords: Object.entries(item.dockerLabels ?? {}).flat(), host: hostName(item.hostId), to: `/containers?c=${encodeURIComponent(item.id)}` })) ?? []} query={query} select={select} loading={containers.isPending} list="/containers" />
    <SearchGroup name={nav("images")} entries={images.data?.map((item) => ({ id: item.id, label: `${item.repository}:${item.tag ?? ""}`, host: hostName(item.hostId), to: `/images?image=${encodeURIComponent(item.id)}` })) ?? []} query={query} select={select} loading={images.isPending} list="/images" />
    <SearchGroup name={nav("volumes")} entries={volumes.data?.map((item) => ({ id: item.id, label: item.name, keywords: Object.entries(item.labels ?? {}).flat(), host: hostName(item.hostId), to: `/volumes/${encodeURIComponent(item.id)}` })) ?? []} query={query} select={select} loading={volumes.isPending} list="/volumes" />
    <SearchGroup name={nav("networks")} entries={topology.data?.networks.map((item) => ({ id: item.id, label: item.name, keywords: Object.entries(item.labels ?? {}).flat(), host: hostName(item.hostId), to: `/networks?host=${encodeURIComponent(item.hostId)}&node=${encodeURIComponent(item.id)}` })) ?? []} query={query} select={select} loading={topology.isPending} list="/networks" />
    <SearchGroup name={t("actions")} entries={[...actions, ...sharedActions]} query={query} select={select} />
  </>;
}
