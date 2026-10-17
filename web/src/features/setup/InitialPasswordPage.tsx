// Source: docklet-next/docs/design/identity.md
// Revision: 5b7a533b0682154acf719fc2ddd059aed9a443c2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original password-change screen for an already configured hub.

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import { useCurrentUser } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";

export function InitialPasswordPage() {
  const t = useTranslations("setup");
  const tp = useTranslations("settings.profile");
  const adapter = useAdapter();
  const user = useCurrentUser();
  const client = useQueryClient();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  return <main className="mx-auto max-w-lg p-6">
    <h1 className="mb-4 text-xl font-semibold">{t("newPassword")}</h1>
    <form className="flex flex-col gap-4" onSubmit={(event) => {
      event.preventDefault();
      if (busy || !current || password.length < 12 || password !== confirm) { setFailed(true); return; }
      setBusy(true);
      setFailed(false);
      void adapter.changePassword(current, password).then(() => {
        client.setQueryData(queryKeys.currentUser, { ...user.data, requiresPasswordChange: false });
      }).catch(() => setFailed(true)).finally(() => setBusy(false));
    }}>
      <TextField id="initial-current" type="password" label={t("currentPassword")} value={current} onChange={setCurrent} autoComplete="current-password" />
      <TextField id="initial-new" type="password" label={t("newPassword")} value={password} onChange={setPassword} autoComplete="new-password" />
      <TextField id="initial-confirm" type="password" label={t("confirmPassword")} value={confirm} onChange={setConfirm} autoComplete="new-password" />
      {failed && <p role="alert">{tp("passwordError")}</p>}
      <Button type="submit" disabled={busy}>{tp(busy ? "passwordSaving" : "password")}</Button>
    </form>
  </main>;
}
