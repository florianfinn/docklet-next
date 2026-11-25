// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231. M19: total contract-code catalogue maps. M21 #439: Compose self-update controls and capability integration. M40 #615: condensed version hints, no agent hint on the hub host, empty orphan section hidden. M41 #624: hints and restore points follow the reported agent version and a settled self-update.

import { SYSTEM_HINT_STATE_KEYS, SYSTEM_HINT_KIND_KEYS } from "@/data/wire-catalogue-states";
import { scopedKey } from "@/data/wire-catalogue";
import { formatDateTime } from "@/domain/format";

import { useEffect, useRef, useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { selfUpdateQueryKey } from "@/shared/components/updates/use-self-update-check";
import type { SelfUpdateCheck } from "@/domain/self-update";
import { useCapability } from "@/data/use-capability";
import { DeletePointDialog } from "@/shared/components/updates/delete-point-dialog";
import { RestorePointState } from "@/shared/components/updates/restore-point-state";
import type { UpdatePoint } from "@/domain/stack-updates";
import type { Host } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { CopyButton } from "@/shared/components/code-block";
import type { SystemHints } from "@/domain/stack-updates";
type SystemImageHint = SystemHints["images"][number];

import { UpdateUnavailable } from "@/shared/components/updates/live-state";
import { useImageError } from "@/features/images/use-image-error";
import { LiveUpdatesSelfUpdate } from "./LiveUpdatesSelfUpdate";

/** Long references keep their name and tag; a digest shows its first 12 hex digits. The full value is copied. */
const shortReference = (ref: string) => ref.replace(/@sha256:([0-9a-f]{12})[0-9a-f]{52}$/, "@sha256:$1…");
function HintReference({ value }: { value: string }) {
  return <span className="inline-flex min-w-0 max-w-full items-center gap-1"><code title={value} className="min-w-0 truncate text-xs">{shortReference(value)}</code><CopyButton text={value} className="h-7 shrink-0" /></span>;
}

export function HostUpdateSettings({ hostId, name, host }: { hostId: string; name: string; host?: Pick<Host, "identity" | "agentVersion"> }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const hintRead = useCapability("listSystemComponents", { kind: "host", id: hostId });
  // Hints belong to the host's registration and reported agent version, like its self-update check:
  // an updated agent selects a fresh entry instead of showing the old version as available.
  const hints = useQuery({ queryKey: ["schedule", hostId, "hints", host?.identity ?? "", host?.agentVersion ?? ""], queryFn: () => adapter.getSystemImageHints!(hostId), enabled: hintRead.available && !!adapter.getSystemImageHints });
  const selfUpdate = useQuery<SelfUpdateCheck>({ queryKey: selfUpdateQueryKey(hostId, host), enabled: false });
  const phase = selfUpdate.data?.operation?.phase, lastPhase = useRef(phase), refetchHints = hints.refetch;
  const [deleting, setDeleting] = useState<{ hostId: string; point: UpdatePoint }>();
  const orphanDelete = useCapability("deleteHostRestorePoint", { kind: "host", id: hostId });
  const orphanRead = useCapability("listHostRestorePoints", { kind: "host", id: hostId });
  // Like the hints, restore points read during an agent restart may have failed; the recovered agent gets a fresh read.
  const orphans = useQuery({ queryKey: ["schedule", hostId, "orphan-points", host?.identity ?? "", host?.agentVersion ?? ""], queryFn: () => adapter.listHostRestorePoints!(hostId), enabled: orphanRead.available && !!adapter.listHostRestorePoints });
  const refetchOrphans = orphans.refetch;
  useEffect(() => {
    // A settled self-update can report the same version (a failed or repeated update): read both once more.
    if (lastPhase.current && lastPhase.current !== "settled" && phase === "settled") { void refetchHints(); void refetchOrphans(); }
    lastPhase.current = phase;
  }, [phase, refetchHints, refetchOrphans]);
  const refreshPoints = async () => { const result = await orphans.refetch(); if (result.isError) throw result.error; return result.data!; };
  // The hub's own Docker runs no agent: its agent hint compares the hub image and can never be checked.
  const hintRows = hints.data?.images.filter((h) => hostId !== "0" || h.kind !== "agent") ?? [];
  const orphaned = orphans.data?.filter((p) => p.orphaned) ?? [];
  return <section className="flex flex-col gap-3 rounded-xl border p-4">
    <LiveUpdatesSelfUpdate key={hostId} hostId={hostId} name={name} host={host} />
    {!hintRead.available ? <UpdateUnavailable reason={hintRead.reason} /> : hints.isError ? <p role="alert">{errorText(hints.error)}</p> : hintRows.length > 0 && <div className="flex flex-col gap-1.5"><h4 className="text-sm font-medium">{t("hints")}</h4><ul className="flex flex-col gap-2">{hintRows.map((h) => <HintRow key={h.kind} hint={h} />)}</ul></div>}
    {orphanRead.available && (orphans.isError || orphaned.length > 0) && <div className="flex flex-col gap-2"><h4 className="text-sm font-medium">{t("orphans")}</h4>{orphans.isError ? <p role="alert">{errorText(orphans.error)}</p> : orphaned.map((p) => <article key={p.id} className="rounded-lg border p-3"><p className="break-all">{p.targetId} · {p.targetIdentity} · {formatDateTime(p.createdAt)}</p><RestorePointState point={p} />{p.code && <p>{errorText(p.code)}</p>}<Button type="button" variant="outline" disabled={!orphanDelete.available || p.holdState !== "none"} onClick={() => setDeleting({ hostId, point: p })}>{t("deletePoint")}</Button></article>)}</div>}
    {deleting?.hostId === hostId && <DeletePointDialog key={`${hostId}:${deleting.point.id}:${deleting.point.targetIdentity}`} scope={{ kind: "host", id: hostId }} point={deleting.point} readError={orphans.isError ? orphans.error : undefined} latest={orphans.data?.find((p) => p.id === deleting.point.id)} refresh={refreshPoints} close={() => setDeleting(undefined)} />}
  </section>;
}

function HintRow({ hint: h }: { hint: SystemImageHint }) {
  const t = useTranslations("common.stackUpdates"), all = useTranslations(), errorText = useImageError();
  const target = h.candidateVersion && h.candidateVersion !== h.currentVersion ? h.candidateVersion : undefined;
  return <li className="flex flex-col gap-1 text-sm">
    <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5"><span className="font-medium">{all(SYSTEM_HINT_KIND_KEYS[h.kind])}</span><span className="font-mono text-xs">{h.currentVersion ?? t("unknown")}{target && ` → ${target}`}</span><span className="text-muted-foreground">· {t(scopedKey(SYSTEM_HINT_STATE_KEYS[h.state], "common.stackUpdates."))}</span>{h.requiresHubUpdate && <span>{t("hubFirst")}</span>}{h.code && <span className="text-muted-foreground">{errorText(h.code)}</span>}</span>
    {h.reference && <HintReference value={h.reference} />}
    {h.compatibleReference && h.compatibleReference !== h.reference && <HintReference value={h.compatibleReference} />}
  </li>;
}
