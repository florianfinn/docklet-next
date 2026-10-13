import { Database, FolderOpen } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useVolumes } from "@/data/queries";
import { formatBytes } from "@/domain/format";
import { volumeLabel } from "@/domain/volumes";
import { IconTile } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";

import { filesHref } from "./files/files-href";
import { BackupBadge, KindBadge } from "./storage-visuals";

/** Volumes and bind mounts used by a stack's services, each linked to its file browser. */
export function StorageCard({ stackId }: { stackId: string }) {
  const t = useTranslations("volumes.storageCard");
  const volumes = useVolumes();
  const used = (volumes.data ?? []).filter((v) => v.consumers.some((c) => c.stackId === stackId));
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center gap-2.5">
          <IconTile tone="teal" size="sm">
            <Database />
          </IconTile>
          {t("title")}
        </CardTitle>
        <CardDescription>{t("description")}</CardDescription>
      </CardHeader>
      <CardContent className="px-4">
        {volumes.isPending ? (
          <Skeleton className="h-24 rounded-xl" />
        ) : used.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("empty")}</p>
        ) : (
          <ul className="grid grid-cols-1 gap-2 md:grid-cols-2">
            {used.map((v) => (
              <li key={v.id} className="flex min-w-0 items-center gap-3 rounded-xl border bg-card/40 p-2.5">
                <div className="min-w-0 flex-1">
                  <Link to={`/volumes/${v.id}`} className="block truncate text-sm font-medium hover:underline">
                    {volumeLabel(v)}
                  </Link>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                    <KindBadge volume={v} className="h-5 px-1.5 text-[10.5px]" />
                    <span className="font-mono">{formatBytes(v.sizeBytes)}</span>
                    <span className="truncate font-mono">
                      → {v.consumers.filter((c) => c.stackId === stackId).map((c) => `${c.service}:${c.target}`).join(", ")}
                    </span>
                  </div>
                </div>
                <BackupBadge volume={v} />
                <Button asChild size="icon" variant="ghost" className="size-8 shrink-0">
                  <Link to={filesHref(v)} aria-label={t("files", { name: volumeLabel(v) })}>
                    <FolderOpen />
                  </Link>
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
