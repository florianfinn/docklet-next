// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original production read-only installation sheet and explicitly confirmed ephemeral token rotation. M14 host maintenance and availability.

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { queryKeys } from "@/data/query-keys";
import { useCapability } from "@/data/use-capability";
import type { Host } from "@/domain/model";
import type { GeneralSettings, HostDefaults } from "@/domain/settings";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { DialogError } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { TextField } from "@/shared/components/form-field";
import { QueryError } from "@/shared/components/page";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Skeleton } from "@/shared/ui/skeleton";
import { FieldGroup } from "@/shared/ui/field";
import { InstallArtifacts } from "./InstallArtifacts";
import { existingHostDefaults, existingHostPlan } from "./existing-host-plan";
import { useHostError } from "./use-host-error";

function InstallSession({ host, defaults, general, image }: { host: Host; defaults: HostDefaults; general: GeneralSettings; image: string | undefined }) {
  const t = useTranslations("hosts.install"), fields = useTranslations("hosts.onboarding");
  const adapter = useAdapter(), formatError = useHostError();
  const feedback = useDialogCommand(true, host.id, formatError);
  const capability = useCapability("issueAgentToken");
  const [draft, setDraft] = useState(() => existingHostDefaults(host, defaults));
  const [token, setToken] = useState<string>();
  const [confirm, setConfirm] = useState(false);
  useEffect(() => identityLifetime(adapter).listen(() => setToken(undefined)), [adapter]);
  let plan: ReturnType<typeof existingHostPlan> | undefined;
  try { plan = existingHostPlan(host, draft, general, token, image); } catch { /* Invalid local fields keep artifacts unmounted. */ }
  const patch = (key: keyof HostDefaults, value: string) => setDraft(old => ({ ...old, [key]: value }));
  return <div {...feedback.bindings} className="flex flex-col gap-4 p-4">
    <p className="text-sm text-muted-foreground">{t("placeholderHint")}</p>
    <FieldGroup>
      <TextField id="install-project" label={fields("projectDir")} value={draft.projectDir} onChange={v => patch("projectDir", v)} mono />
      <TextField id="install-directory" label={fields("installDir")} value={draft.installDir} onChange={v => patch("installDir", v)} mono />
      <div className="grid grid-cols-2 gap-3">
        <TextField id="install-puid" label={fields("puid")} value={draft.puid} onChange={v => patch("puid", v)} mono />
        <TextField id="install-pgid" label={fields("pgid")} value={draft.pgid} onChange={v => patch("pgid", v)} mono />
      </div>
    </FieldGroup>
    <DialogError feedback={feedback} />
    {plan ? <InstallArtifacts plan={plan} onRegenerateToken={!token && capability.available ? () => setConfirm(true) : undefined} /> : <p role="alert" className="text-sm text-destructive">{t("invalidFields")}</p>}
    <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("generateTitle", { name: host.name })} description={t("generateWarning")} confirmLabel={t("generate")} formatError={formatError} onConfirm={async () => {
      const current = feedback.checkpoint();
      const issued = await adapter.issueAgentToken(host.id);
      if (!current() || !issued) return false;
      setToken(issued); return true;
    }} />
  </div>;
}
export default function InstallSheet({ host, onClose }: { host: Host | null; onClose: () => void }) {
  const t = useTranslations("hosts.install"), adapter = useAdapter();
  const settings = useQuery({ queryKey: [...queryKeys.connectedSettings, "installation"], queryFn: () => adapter.getConnectedSettings("installation"), enabled: !!host });
  const discovery = useQuery({ queryKey: ["hub", "discovery"], queryFn: () => adapter.discoverHub(), enabled: !!host });
  return <Sheet open={!!host} onOpenChange={open => { if (!open) onClose(); }}>
    <SheetContent className="w-full gap-0 p-0 sm:max-w-2xl">
      <SheetHeader className="border-b"><SheetTitle>{t("sheetTitle", { name: host?.name ?? "" })}</SheetTitle><SheetDescription>{t("sheetDescription")}</SheetDescription></SheetHeader>
      <ScrollArea className="min-h-0 flex-1">
        {host && (settings.isError ? <QueryError error={settings.error} onRetry={() => void settings.refetch()} /> : discovery.isError ? <QueryError error={discovery.error} onRetry={() => void discovery.refetch()} /> : settings.data && discovery.data && (adapter.kind !== "http" || discovery.data.agentImage) ? <InstallSession key={host.id} host={host} defaults={settings.data.hostDefaults} general={settings.data.general} image={discovery.data.agentImage} /> : <Skeleton className="m-4 h-80 rounded-xl" />)}
      </ScrollArea>
    </SheetContent>
  </Sheet>;
}
