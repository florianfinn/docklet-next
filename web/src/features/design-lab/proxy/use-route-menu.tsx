import { Copy, FileCode2, Pencil, ScanSearch, Trash2 } from "lucide-react";
import { useNavigate } from "react-router";
import type { MenuGroups } from "@/shared/components/action-menu";
import type { RouteActionsProps } from "./RouteActions";
import { useProxyText } from "./use-proxy-text";
export function useRouteMenu({ route, onEdit, onInspect, onDuplicate, onRemove }: RouteActionsProps): MenuGroups {
  const t = useProxyText();
  const navigate = useNavigate();
  return [[route.source === "file" ? { id: "edit", label: t("edit"), icon: <Pencil />, onSelect: onEdit } : { id: "compose", label: t("openCompose"), icon: <FileCode2 />, onSelect: () => navigate("/stacks/" + route.stackId + "?tab=editor") },
    { id: "details", label: t("openDetails"), icon: <ScanSearch />, onSelect: onInspect },
    { id: "duplicate", label: t("duplicate"), icon: <Copy />, onSelect: onDuplicate }],
    route.source === "file" ? [{ id: "delete", label: t("remove"), icon: <Trash2 />, onSelect: onRemove }] : []];
}
