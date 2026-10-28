// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original asynchronous replace/new restore confirmation.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { backupErrorMessage } from "@/data/backup-error";
import type { Volume } from "@/domain/volumes";
import { ChoiceCards } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";

// The suffix follows the operator's calendar day, not UTC.
const localDay = (d: Date) => `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, "0")}${String(d.getDate()).padStart(2, "0")}`;

export function RestoreDialog({ volume, runId, file, onClose, accepted }: { volume: Volume; runId?: string; file?: File; onClose: () => void; accepted: (id: string) => void }) {
  const t = useTranslations("volumes.backups"), all = useTranslations(), adapter = useAdapter();
  const [mode, setMode] = useState<"replace" | "new">("replace"), [name, setName] = useState(`${volume.name}-restore-${localDay(new Date())}`);
  const [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  return <Dialog open onOpenChange={(open) => !open && !busy && onClose()}><DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl"><DialogHeader><DialogTitle>{file ? t("uploadRestore") : t("restore")}</DialogTitle><DialogDescription>{t("restoreBody")}</DialogDescription></DialogHeader>
    <ChoiceCards label={t("restore")} value={mode} onChange={setMode} columns={2} options={[{ value: "replace", title: t("restoreReplace"), description: volume.consumers.some((c) => c.running) ? t("restoreReplaceHint", { names: volume.consumers.filter((c) => c.running).map((c) => c.containerName).join(", ") }) : t("restoreReplaceHintIdle") }, { value: "new", title: t("restoreNew"), description: t("restoreNewHint") }]} />
    {mode === "new" && <Field><FieldLabel htmlFor="restore-name">{t("newVolumeName")}</FieldLabel><Input id="restore-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    <DialogFooter><Button variant="outline" disabled={busy} onClick={onClose}>{t("cancel")}</Button><Button disabled={busy || mode === "new" && !/^[a-zA-Z0-9][a-zA-Z0-9_.-]+$/.test(name)} onClick={async () => { setBusy(true); setError(undefined); try { const id = file ? await adapter.uploadRestore(volume.id, file, mode, mode === "new" ? name : undefined) : await adapter.restoreBackup(volume.id, runId!, mode, mode === "new" ? name : undefined); accepted(id); onClose(); } catch (cause) { setError(backupErrorMessage(cause, all)); } finally { setBusy(false); } }}>{t("restoreConfirm")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
