// Source: docklet-next/issues/324
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original personal-key profile with a transient creation secret.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslations } from "use-intl";
import type { PersonalApiKey } from "@/data/api";
import { useAdapter } from "@/data/use-adapter";
import type { AuthMode } from "@/domain/settings";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { TextField } from "@/shared/components/form-field";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";

export function PersonalApiKeys({ mode }: { mode: AuthMode }) {
  const t = useTranslations("settings.personalKeys");
  return <Card className="gap-4 py-5"><CardHeader className="px-5"><CardTitle>{t("title")}</CardTitle></CardHeader><CardContent className="flex flex-col gap-4 px-5">{mode === "local" ? <KeyList /> : <p className="text-sm text-muted-foreground">{t("unavailable")}</p>}</CardContent></Card>;
}

function KeyList() {
  const t = useTranslations("settings.personalKeys");
  const adapter = useAdapter();
  const keys = useQuery({ queryKey: ["personal-api-keys"], queryFn: () => adapter.listMyApiKeys() });
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<PersonalApiKey | null>(null);
  const saved = () => { void keys.refetch(); };
  return <>
    <p className="text-xs text-muted-foreground">{t("hint")}</p>
    <Button className="self-start" size="sm" onClick={() => setCreating(true)}>{t("add")}</Button>
    {keys.isError ? <QueryError error={keys.error} onRetry={saved} /> : keys.isPending ? <p aria-busy="true">{t("loading")}</p> : keys.data.length === 0 ? <p className="text-sm text-muted-foreground">{t("empty")}</p> : <div className="overflow-x-auto"><Table>
      <TableHeader><TableRow>{(["name", "prefix", "expiry", "lastUse", "revoke"] as const).map((key) => <TableHead key={key}>{t(key)}</TableHead>)}</TableRow></TableHeader>
      <TableBody>{keys.data.map((key) => <TableRow key={key.id}><TableCell>{key.name}</TableCell><TableCell className="font-mono text-xs">{key.keyPrefix}</TableCell><TableCell>{key.expiresAt ?? t("noExpiry")}</TableCell><TableCell>{key.lastUsedAt ?? t("never")}</TableCell><TableCell><Button variant="ghost" size="sm" onClick={() => setRevoking(key)}>{t("revoke")}</Button></TableCell></TableRow>)}</TableBody>
    </Table></div>}
    {creating && <CreateKeyDialog secret={secret} onSecret={setSecret} onClose={() => { setSecret(null); setCreating(false); }} onSaved={saved} />}
    {revoking && <ConfirmDialog open onOpenChange={(open) => { if (!open) setRevoking(null); }} title={t("confirm", { name: revoking.name })} description={t("revokeHint")} confirmLabel={t("revoke")} destructive onConfirm={async () => { await adapter.deleteMyApiKey(revoking.id); saved(); }} />}
  </>;
}

function CreateKeyDialog({ onClose, onSaved, secret, onSecret }: { onClose: () => void; onSaved: () => void; secret: string | null; onSecret: (value: string) => void }) {
  const t = useTranslations("settings.personalKeys");
  const adapter = useAdapter();
  const [name, setName] = useState("");
  const [expiry, setExpiry] = useState("");
  const [openedAt] = useState(Date.now);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [copied, setCopied] = useState(false);
  const close = () => { if (busy) return; onClose(); };
  const save = async () => {
    if (expiry && Date.parse(`${expiry}T00:00:00Z`) <= Date.now()) { setError(true); return; }
    setBusy(true); setError(false);
    try {
      const key = await adapter.createMyApiKey({ name: name.trim(), ...(expiry ? { expiresAt: new Date(`${expiry}T00:00:00Z`).toISOString() } : {}) });
      onSecret(key.key); onSaved();
    } catch { setError(true); }
    finally { setBusy(false); }
  };
  return <Dialog open onOpenChange={(open) => { if (!open) close(); }}><DialogContent className="sm:max-w-lg">
    <DialogHeader><DialogTitle>{t(secret ? "revealTitle" : "add")}</DialogTitle><DialogDescription>{t(secret ? "revealHint" : "hint")}</DialogDescription></DialogHeader>
    {secret ? <><code className="break-all rounded-lg border p-3 text-sm" data-testid="api-key-secret">{secret}</code><DialogFooter><Button variant="outline" onClick={() => { void Promise.resolve().then(() => navigator.clipboard.writeText(secret)).then(() => setCopied(true), () => setError(true)); }}>{t(copied ? "copied" : "copy")}</Button><Button onClick={close}>{t("close")}</Button></DialogFooter></> : <form className="flex flex-col gap-4" onSubmit={(e) => { e.preventDefault(); void save(); }}>
      <fieldset disabled={busy} className="flex flex-col gap-4"><TextField id="key-name" label={t("name")} value={name} onChange={setName} /><TextField id="key-expiry" label={t("expiry")} type="date" value={expiry} onChange={setExpiry} /><p className="text-xs text-muted-foreground">{t("expiryHint")}</p></fieldset>
      <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={close}>{t("cancel")}</Button><Button type="submit" disabled={busy || !name.trim() || (!!expiry && Date.parse(`${expiry}T00:00:00Z`) <= openedAt)}>{t("create")}</Button></DialogFooter>
    </form>}
    {error && <p role="alert">{t("error")}</p>}
  </DialogContent></Dialog>;
}
