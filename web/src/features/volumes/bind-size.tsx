// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original bind size card: newest of scan and snapshot size, scan on demand.

import { useState } from "react";
import { useTranslations } from "use-intl";

import { backupErrorMessage } from "@/data/backup-error";
import { useAdapter } from "@/data/use-adapter";
import { formatBytes, formatRelative } from "@/domain/format";
import { bindSize, type Volume } from "@/domain/volumes";
import { Button } from "@/shared/ui/button";
import { useVolumeCapability } from "./use-volume-capability";

/** Size of a bind with its date and origin; the scan reads the host path and needs the files read right. */
export function BindSize({ volume, online, onMeasured }: { volume: Volume; online: boolean; onMeasured: () => void }) {
  const t = useTranslations("volumes.detail");
  const all = useTranslations();
  const adapter = useAdapter();
  const read = useVolumeCapability("listFiles", volume.id);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const size = bindSize(volume.bind);
  return (
    <div className="flex flex-col gap-2">
      <div className="text-2xl font-semibold tracking-tight tabular">{size ? formatBytes(size.bytes) : t("sizeUnknown")}</div>
      <p className="text-[11px] text-muted-foreground">
        {size ? t(size.source === "measured" ? "sizeMeasured" : "sizeSnapshot", { time: formatRelative(size.at) }) : t("sizeNone")}
      </p>
      {size?.partial && <p className="text-[11px] text-muted-foreground">{t("sizePartial")}</p>}
      {adapter.measureBind && volume.bind?.eligible && (
        <>
          <Button
            variant="outline"
            size="sm"
            className="self-start"
            disabled={!online || busy || !read.available}
            title={read.reason}
            onClick={async () => {
              setBusy(true);
              setError(undefined);
              try {
                await adapter.measureBind!(volume.id);
                onMeasured();
              } catch (cause) {
                setError(backupErrorMessage(cause, all));
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy ? t("measuring") : t("measure")}
          </Button>
          <p className="text-[11px] text-muted-foreground">{t("measureHint")}</p>
        </>
      )}
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
