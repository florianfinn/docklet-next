import { Archive, Eye, FolderOpen, Layers, Play, Trash2 } from "lucide-react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import type { Host } from "@/domain/model";
import type { Volume } from "@/domain/volumes";
import type { MenuGroups } from "@/shared/components/action-menu";

import { filesHref } from "./files/files-href";

/** Why "Löschen" is unavailable, or undefined when the volume can be deleted now. */
export function deleteBlock(v: Pick<Volume, "kind" | "consumers">, host: Pick<Host, "status"> | undefined): "bind" | "in-use" | "offline" | undefined {
  if (v.kind === "bind") return "bind";
  if (v.consumers.length) return "in-use";
  if (host?.status !== "online") return "offline";
  return undefined;
}

/** Actions of one volume; the ellipsis button and the row's context menu share them. */
export function useVolumeMenu(v: Volume, host: Host | undefined, on: { remove: (v: Volume) => void; backup: (v: Volume) => void }): MenuGroups {
  const t = useTranslations("volumes.menu");
  const navigate = useNavigate();
  const online = host?.status === "online";
  return [
    [
      { id: "open", label: t("open"), icon: <Eye />, onSelect: () => navigate(`/volumes/${v.id}`) },
      { id: "files", label: t("files"), icon: <FolderOpen />, disabled: !online, onSelect: () => navigate(filesHref(v)) },
      { id: "backups", label: t("backups"), icon: <Archive />, onSelect: () => navigate(`/volumes/${v.id}?tab=backups`) },
    ],
    [
      ...(v.stackId ? [{ id: "stack", label: t("stack"), icon: <Layers />, onSelect: () => navigate(`/stacks/${v.stackId}`) }] : []),
      { id: "backup", label: t("backupNow"), icon: <Play />, disabled: !online || !v.backup, onSelect: () => on.backup(v) },
    ],
    v.kind === "volume" ? [{ id: "delete", label: t("delete"), icon: <Trash2 />, disabled: !!deleteBlock(v, host), onSelect: () => on.remove(v) }] : [],
  ];
}
