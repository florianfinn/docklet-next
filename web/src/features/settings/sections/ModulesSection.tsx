// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original administrator module settings and confirmed data deletion.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCommandError } from "@/data/use-command-error";
import { useIsUser } from "@/data/use-principal";
import { identityLifetime } from "@/data/identity-lifetime";
import { useIdentityVersion } from "@/data/use-module";
import { MODULES, defaultModules, type ModuleChoices, type ModuleID, type ModuleDeleteResult } from "@/domain/modules";
import { QueryError } from "@/shared/components/page";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Switch } from "@/shared/ui/switch";
import { Field, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/shared/ui/card";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Skeleton } from "@/shared/ui/skeleton";
import { Button } from "@/shared/ui/button";

export function ModulesSection() {
  const version = useIdentityVersion();
  return <ModuleManagement key={version} />;
}

function ModuleManagement() {
  const adapter = useAdapter(), t = useTranslations("settings.modules"), errorText = useCommandError();
  const client = useQueryClient(), user = useIsUser(), version = useIdentityVersion();
  const [confirm, setConfirm] = useState<{ id: ModuleID; action: "disable" | "delete" } | null>(null);
  const [result, setResult] = useState<{ version: number; value: ModuleDeleteResult }>();
  const [failure, setFailure] = useState<unknown>(), [busy, setBusy] = useState(false);
  const query = useQuery({ queryKey: ["module-management", version], queryFn: () => adapter.getModules!(), enabled: !user && !!adapter.getModules });
  if (user) return null;
  const refresh = async () => { await Promise.all([client.invalidateQueries({ queryKey: ["module-states"] }), client.invalidateQueries({ queryKey: ["module-management"] })]); };
  const toggle = async (id: ModuleID, enabled: boolean) => {
    const owner = identityLifetime(adapter).version(); setBusy(true); setFailure(undefined);
    try {
      await adapter.setModuleEnabled!(id, enabled);
      if (owner === identityLifetime(adapter).version()) {
        // Publish the confirmed toggle before refetching so module surfaces and
        // reads disappear even if the public-settings response is delayed.
        client.setQueryData<ModuleChoices>(["module-states", owner], (previous) => ({ ...(previous ?? defaultModules()), [id]: enabled }));
        await refresh();
      }
    }
    catch (error) { if (owner === identityLifetime(adapter).version()) setFailure(error); throw error; }
    finally { setBusy(false); }
  };
  return <Card>
    <CardHeader><CardTitle><h2>{t("title")}</h2></CardTitle><CardDescription>{t("lead")}</CardDescription></CardHeader>
    <CardContent className="flex flex-col gap-4">
    <FieldGroup>
    {query.isError ? <QueryError error={query.error} onRetry={() => void query.refetch()} /> : !query.data ? <div aria-busy="true"><span className="sr-only">{t("loading")}</span><Skeleton className="h-16 rounded-xl" /></div> : MODULES.map((m) => {
      const enabled = query.data.find((row) => row.id === m.id)?.enabled === true;
      return <Field key={m.id} orientation="horizontal" data-disabled={busy || undefined} className="flex-wrap justify-between rounded-xl border p-4">
        <FieldLabel htmlFor={`module-${m.id}`}>{t(`names.${m.id}`)}</FieldLabel>
        <Switch id={`module-${m.id}`} checked={enabled} disabled={busy || !adapter.setModuleEnabled} onCheckedChange={(on) => { if (!on) setConfirm({ id: m.id, action: "disable" }); else void toggle(m.id, true).catch(() => {}); }} />
        <Button variant="outline" disabled={enabled || busy || !adapter.deleteModuleData} onClick={() => { setResult(undefined); setConfirm({ id: m.id, action: "delete" }); }}>{t("delete")}</Button>
      </Field>;
    })}
    </FieldGroup>
    {failure !== undefined && <Alert variant="destructive"><AlertDescription>{errorText(failure)}</AlertDescription></Alert>}
    {result?.version === version && <Alert role="status"><AlertDescription>{result.value.pendingCleanup.length ? <><p>{t("pending")}</p><ul>{result.value.pendingCleanup.map((host) => <li key={host.id}>{host.name || host.id}</li>)}</ul></> : t("deleted")}</AlertDescription></Alert>}
    <ConfirmDialog key={version} open={confirm !== null} onOpenChange={(open) => { if (!open) setConfirm(null); }} title={t(confirm?.action === "delete" ? "delete" : "disable")} description={t(confirm?.action === "delete" ? "deleteConfirm" : "disableConfirm")} confirmLabel={t(confirm?.action === "delete" ? "delete" : "disable")} destructive formatError={errorText} onConfirm={async () => {
      if (!confirm) return;
      if (confirm.action === "disable") await toggle(confirm.id, false);
      else { const owner = identityLifetime(adapter).version(); const value = await adapter.deleteModuleData!(confirm.id); if (owner === identityLifetime(adapter).version()) setResult({ version: owner, value }); }
    }} />
  </CardContent></Card>;
}
