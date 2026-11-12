// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useRouteMenu } from "./use-route-menu";
import { ActionsMenu } from "@/shared/components/action-menu";
import type { ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
export interface RouteActionsProps { route: ProxyRoute; onEdit: () => void; onInspect: () => void; onDuplicate: () => void; onRemove: () => void }
export function RouteActions(props: RouteActionsProps) {
  const t = useProxyText();
  const menu = useRouteMenu(props);
  return <ActionsMenu groups={menu} label={t("moreActions") + " · " + props.route.domain} />;
}
