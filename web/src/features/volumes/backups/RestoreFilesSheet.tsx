// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original paginated snapshot selection; reads are owned by the open folder.

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { backupErrorMessage } from "@/data/backup-error";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";

export function RestoreFilesSheet({ volumeId, runId, onClose, accepted }: { volumeId: string; runId: string; onClose: () => void; accepted: (id: string) => void }) {
  const adapter = useAdapter(), t = useTranslations("volumes.backups"), all = useTranslations();
  const [path, setPath] = useState("/"), [start, setStart] = useState(0), [paths, setPaths] = useState<string[]>([]), [confirm, setConfirm] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  const listing = useQuery({ queryKey: ["backup-browser", volumeId, runId, path, start], queryFn: () => adapter.browseBackup(volumeId, runId, path, start), retry: false, gcTime: 0 });
  const move = (next: string) => { setPath(next); setStart(0); setConfirm(false); };
  return <Sheet open onOpenChange={(open) => !open && !busy && onClose()}><SheetContent className="flex w-full flex-col overflow-y-auto sm:max-w-xl"><SheetHeader><SheetTitle>{t("restoreFiles")}</SheetTitle><SheetDescription>{t("restoreFilesHint")}</SheetDescription></SheetHeader><div className="space-y-3 px-4 pb-4">
    <div className="flex items-center gap-2"><Button variant="outline" disabled={path === "/"} onClick={() => move(path.split("/").slice(0, -1).join("/") || "/")}>{t("parentFolder")}</Button><code className="break-all text-xs">{path}</code></div>
    {listing.isError ? <p role="alert">{backupErrorMessage(listing.error, all)}</p> : listing.isPending ? <p role="status">{t("following")}</p> : <><ul className="space-y-2">{(listing.data.data ?? []).map((entry) => <li key={entry.path} className="flex items-center gap-3"><Checkbox id={`entry-${entry.path}`} checked={paths.includes(entry.path)} onCheckedChange={(checked) => { setPaths((old) => checked ? [...old, entry.path] : old.filter((p) => p !== entry.path)); setConfirm(false); }} /><label htmlFor={`entry-${entry.path}`} className="min-w-0 flex-1 break-all text-sm">{entry.name}</label>{entry.isDirectory && <Button size="sm" variant="outline" onClick={() => move(entry.path)}>{t("openFolder")}</Button>}</li>)}</ul><div className="flex gap-2"><Button variant="outline" disabled={start === 0} onClick={() => setStart(Math.max(0, start - 100))}>{t("previous")}</Button><Button variant="outline" disabled={start + (listing.data.data?.length ?? 0) >= listing.data.pagination.totalItems} onClick={() => setStart(start + 100)}>{t("nextPage")}</Button></div></>}
    <p className="text-sm">{t("selectedPaths", { count: paths.length })}</p>{confirm && <p>{t("restoreFilesConfirm")}</p>}{error && <p role="alert">{error}</p>}
    <Button disabled={!paths.length || busy} onClick={async () => { if (!confirm) { setConfirm(true); return; } setBusy(true); setError(undefined); try { accepted(await adapter.restoreBackupFiles(volumeId, runId, { paths })); onClose(); } catch (cause) { setError(backupErrorMessage(cause, all)); } finally { setBusy(false); } }}>{confirm ? t("restoreConfirm") : t("restoreFiles")}</Button>
  </div></SheetContent></Sheet>;
}
