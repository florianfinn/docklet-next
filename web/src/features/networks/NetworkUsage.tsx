// Source: docklet-next/issues/204
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Accessible usage indicator; preserve built-in and Compose protection when usage is unknown.

import { CircleDashed, CircleHelp, Link2, Shield } from "lucide-react";
import { useTranslations } from "use-intl";

import { networkComposeOwner } from "@/domain/networks";
import type { ContainerInfo, TopologyNetwork } from "@/domain/model";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export type NetworkMember = Pick<ContainerInfo, "id" | "name" | "state">;

/** Attachments count even when a stopped container has no address or inventory detail. */
export function NetworkUsage({ network, members }: { network: TopologyNetwork; members: NetworkMember[] }) {
  const t = useTranslations("networks.usage");
  const owner = networkComposeOwner(network);
  const unknown = network.detailUnavailable || network.inventory?.stale;
  const protection = network.builtIn ? t("builtin") : owner ? t("stack", { name: owner }) : undefined;
  const state = network.builtIn ? "builtin" : unknown ? "unknown" : members.length ? "used" : "unused";
  const Icon = network.builtIn || owner ? Shield : unknown ? CircleHelp : members.length ? Link2 : CircleDashed;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button type="button" aria-label={unknown && protection ? t("protectedLabel", { name: network.name, reason: protection }) : unknown ? t("unknownLabel", { name: network.name }) : t("label", { name: network.name, state: t(state), count: members.length })} className="inline-flex items-center gap-1 rounded-sm p-1 outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <Icon className="size-3.5" aria-hidden />
          <span>{unknown ? protection ? "–" : "?" : members.length}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-72 break-words">
        <p>{unknown && protection ? protection : t(state)}</p>
        {unknown && protection && <p>{t("unknown")}</p>}
        {network.builtIn && <p>{t("builtinDetail")}</p>}
        {owner && !unknown && <p>{t("stack", { name: owner })}</p>}
        {members.length ? (
          <ul>
            {members.map((c) => <li key={c.id}>{t("member", { name: c.name, state: t(`states.${c.state}`) })}</li>)}
          </ul>
        ) : !unknown && <p>{t("noAttachments")}</p>}
      </TooltipContent>
    </Tooltip>
  );
}
