// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original connected dialog following the approved AuthSection composition.

import { useState } from "react";
import { useTranslations } from "use-intl";
import type { ManagedUser } from "@/data/api";
import { useAdapter } from "@/data/use-adapter";
import type { AuthMode } from "@/domain/settings";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Switch } from "@/shared/ui/switch";

export function UserDialog({ user, mode, currentId, lastAdmin, reset = false, onClose, onSaved }: { user: ManagedUser | null; mode: AuthMode; currentId?: string; lastAdmin: boolean; reset?: boolean; onClose: () => void; onSaved: () => void }) {
  const t = useTranslations("settings.liveUsers");
  const adapter = useAdapter();
  const [name, setName] = useState(user?.username ?? "");
  const [password, setPassword] = useState("");
  const [admin, setAdmin] = useState(user?.isAdmin ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const own = !!user && user.id === currentId;
  const adminReason = own ? t("ownAdmin") : lastAdmin ? t("lastAdmin") : undefined;
  const needsPassword = mode === "local" && (!user || reset);
  const save = async () => {
    setBusy(true); setError(false);
    try {
      if (reset && user) await adapter.resetUserPassword(user.id, password);
      else if (!user) await adapter.createUser({ username: name.trim(), ...(needsPassword ? { password } : {}) });
      else {
        if (name.trim() !== user.username) await adapter.updateUser(user.id, { username: name.trim() });
        if (admin !== user.isAdmin) await adapter.setUserAdministrator(user.id, admin);
      }
      onSaved(); onClose();
    } catch { setError(true); onSaved(); }
    finally { setBusy(false); }
  };
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose(); }}>
    <DialogContent className="sm:max-w-lg">
      <form className="flex flex-col gap-4" onSubmit={(e) => { e.preventDefault(); void save(); }}>
        <DialogHeader><DialogTitle>{reset ? t("resetTitle", { name: user!.username }) : user ? t("editTitle", { name: user.username }) : t("createTitle")}</DialogTitle><DialogDescription>{t(mode === "proxy" ? "proxyHint" : "description")}</DialogDescription></DialogHeader>
        <fieldset disabled={busy} className="flex flex-col gap-4">
          {!reset && <TextField id="user-name" label={t("name")} value={name} onChange={setName} autoComplete="off" />}
          {needsPassword && <><TextField id="user-password" label={t("initialValue")} type="password" value={password} onChange={setPassword} autoComplete="new-password" /><p className="text-xs text-muted-foreground">{t("localHint")}</p></>}
          {user && !reset && <div className="flex flex-col gap-2"><Label htmlFor="user-admin"><Switch id="user-admin" checked={admin} disabled={own || lastAdmin} onCheckedChange={setAdmin} />{t("administrator")}</Label>{adminReason && <p className="text-xs text-muted-foreground">{adminReason}</p>}</div>}
        </fieldset>
        {error && <p role="alert" className="text-sm text-destructive">{t("error")}</p>}
        <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={onClose}>{t("cancel")}</Button><Button type="submit" disabled={busy || !name.trim() || (needsPassword && password.length < 12)}>{t("save")}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
