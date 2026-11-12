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
