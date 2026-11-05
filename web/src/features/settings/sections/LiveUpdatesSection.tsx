// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { formatDateTime } from "@/domain/format";

import { useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { DeletePointDialog } from "@/shared/components/updates/delete-point-dialog";
import { RestorePointState } from "@/shared/components/updates/restore-point-state";
import type { UpdatePoint } from "@/domain/stack-updates";
import { Button } from "@/shared/ui/button";

import { UpdateUnavailable } from "@/shared/components/updates/live-state";
import { useImageError } from "@/features/images/use-image-error";

export function HostUpdateSettings({ hostId, name }: { hostId: string; name: string }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const hintRead = useCapability("listSystemComponents", { kind: "host", id: hostId });
  const hints = useQuery({ queryKey: ["schedule", hostId, "hints"], queryFn: () => adapter.getSystemImageHints!(hostId), enabled: hintRead.available && !!adapter.getSystemImageHints });
  const [deleting, setDeleting] = useState<{ hostId: string; point: UpdatePoint }>();
  const orphanDelete = useCapability("deleteHostRestorePoint", { kind: "host", id: hostId });
  const orphanRead = useCapability("listHostRestorePoints", { kind: "host", id: hostId });
  const orphans = useQuery({ queryKey: ["schedule", hostId, "orphan-points"], queryFn: () => adapter.listHostRestorePoints!(hostId), enabled: orphanRead.available && !!adapter.listHostRestorePoints });
  const refreshPoints = async () => { const result = await orphans.refetch(); if (result.isError) throw result.error; return result.data!; };
  return <section className="flex flex-col gap-3 rounded-xl border p-4"><h3 className="font-semibold">{name}</h3>
    {!hintRead.available ? <UpdateUnavailable reason={hintRead.reason} /> : hints.isError ? <p role="alert">{errorText(hints.error)}</p> : hints.data && <div><h4>{t("hints")}</h4><p>{t("hubVersion", { version: hints.data.hubVersion })}</p>{hints.data.images.map((h) => <p key={h.kind}>{h.kind}: <code>{h.reference}</code> {h.compatibleReference && <code>{h.compatibleReference}</code>} {h.currentVersion ?? t("unknown")} · {t(`hint.${h.state}`)} {h.candidateVersion} {h.requiresHubUpdate && t("hubFirst")} {h.code && errorText(h.code)}</p>)}</div>}
    <h4>{t("orphans")}</h4>{!orphanRead.available ? <UpdateUnavailable reason={orphanRead.reason} /> : orphans.isError ? <p role="alert">{errorText(orphans.error)}</p> : orphans.data?.filter((p) => p.orphaned).map((p) => <article key={p.id} className="rounded-lg border p-3"><p className="break-all">{p.targetId} · {p.targetIdentity} · {formatDateTime(p.createdAt)}</p><RestorePointState point={p} />{p.code && <p>{errorText(p.code)}</p>}<Button type="button" variant="outline" disabled={!orphanRead.available || !orphanDelete.available || p.holdState !== "none"} onClick={() => setDeleting({ hostId, point: p })}>{t("deletePoint")}</Button></article>)}
    {deleting?.hostId === hostId && <DeletePointDialog key={`${hostId}:${deleting.point.id}:${deleting.point.targetIdentity}`} scope={{ kind: "host", id: hostId }} point={deleting.point} readError={orphans.isError ? orphans.error : undefined} latest={orphans.data?.find((p) => p.id === deleting.point.id)} refresh={refreshPoints} close={() => setDeleting(undefined)} />}
  </section>;
}
