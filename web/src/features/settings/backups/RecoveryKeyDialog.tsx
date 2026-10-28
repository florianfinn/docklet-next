// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original recovery key confirmation; values live only inside this mounted dialog.

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { backupErrorMessage } from "@/data/backup-error";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { TextField } from "@/shared/components/form-field";

export function RecoveryKeyDialog({ id, hostId, name, onClose }: { id: string; hostId?: string; name: string; onClose: () => void }) {
  const adapter = useAdapter(), t = useTranslations("settings.backups"), all = useTranslations();
  const [key, setKey] = useState(""), [attach, setAttach] = useState(""), [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  const active = useRef(true);
  useEffect(() => { active.current = true; const off = identityLifetime(adapter).listen(() => { active.current = false; setKey(""); setAttach(""); onClose(); }); return () => { active.current = false; off(); }; }, [adapter, onClose]);
  const close = () => { active.current = false; setKey(""); setAttach(""); onClose(); };
  return <Dialog open onOpenChange={(open) => !open && close()}><DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl"><DialogHeader><DialogTitle>{t("recoveryKey")}: {name}</DialogTitle><DialogDescription>{t("keyWarning")}</DialogDescription></DialogHeader>
    {id === "local" && <p className="text-sm text-muted-foreground">{t("localKeyHint")}</p>}
    {key ? <><output className="break-all rounded-lg border p-3 font-mono text-sm">{key}</output><div className="flex flex-wrap gap-2"><Button onClick={async () => { try { await navigator.clipboard.writeText(key); } catch { setError(t("copyFailed")); } }}>{t("copyKey")}</Button><Button onClick={() => { const url = URL.createObjectURL(new Blob([key + "\n"], { type: "text/plain" })); const link = document.createElement("a"); link.href = url; link.download = "docklet-backup-recovery-key.txt"; link.click(); URL.revokeObjectURL(url); }}>{t("downloadKey")}</Button></div></> : <Button disabled={busy} onClick={async () => { setBusy(true); setError(undefined); try { const value = await adapter.getBackupRecoveryKey(id, hostId); if (active.current) setKey(value); } catch (cause) { if (active.current) setError(backupErrorMessage(cause, all)); } finally { if (active.current) setBusy(false); } }}>{t("confirmKey")}</Button>}
    {id === "local" && hostId && <div className="space-y-2 border-t pt-3"><TextField id="attach-key" type="password" label={t("attachKey")} value={attach} onChange={setAttach} autoComplete="off" description={t("attachKeyHint")} /><Button variant="outline" disabled={!attach || busy} onClick={async () => { setBusy(true); setError(undefined); try { await adapter.attachLocalBackupRecoveryKey(hostId, attach); if (active.current) { setAttach(""); setKey(""); } } catch (cause) { if (active.current) setError(backupErrorMessage(cause, all)); } finally { if (active.current) setBusy(false); } }}>{t("attachKey")}</Button></div>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}<DialogFooter><Button variant="outline" onClick={close}>{t("close")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
