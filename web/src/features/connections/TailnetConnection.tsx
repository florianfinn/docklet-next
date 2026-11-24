// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers. M37 #586: setup steps (policy first, credential, save, test).

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Network } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";
import { useHosts } from "@/data/queries";
import { useTailnet } from "@/data/use-tailnet";
import { useCommandError } from "@/data/use-command-error";
import type { TailnetSettings } from "@/domain/tailnet";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { Input } from "@/shared/ui/input";
import { Button } from "@/shared/ui/button";
import { CredentialStep, PolicyStep, Step, TestStep } from "./TailnetSetupSteps";

/** One write-only credential form shared by first-run setup and Connections. */
export function TailnetConnection({ setup = false, onContinue, onSkip }: { setup?: boolean; onContinue?: () => void; onSkip?: () => void }) {
  const t = useTranslations("connections");
  const wire = useTranslations("tailnetContract");
  const { api, status, query } = useTailnet();
  const settings = useQuery({ queryKey: ["tailnet-settings"], queryFn: () => api!.settings(), enabled: !!api, retry: false });
  if (!api) return <div className="min-w-0"><p>{t("unsupported")}</p>{onSkip && <Button className="mt-3 min-h-10" onClick={onSkip}>{t("skip")}</Button>}</div>;
  if (query.isError || settings.isError) return <div role="alert"><p>{t("loadError")}</p><Button className="min-h-10" onClick={() => { void query.refetch(); void settings.refetch(); }}>{t("retry")}</Button>{onSkip && <Button className="min-h-10" onClick={onSkip}>{t("skip")}</Button>}</div>;
  if (!settings.data || !status) return <p role="status">{t("loading")}</p>;
  return <TailnetForm key={settings.data.mode ?? "oauth"} settings={settings.data} setup={setup} onContinue={onContinue} onSkip={onSkip} connectionLabel={wire(`connection.${status.connection.state}`)} />;
}

function TailnetForm({ settings, setup, onContinue, onSkip, connectionLabel }: { settings: TailnetSettings; setup: boolean; onContinue?: () => void; onSkip?: () => void; connectionLabel: string }) {
  const t = useTranslations("connections");
  const wire = useTranslations("tailnetContract");
  const { api, status } = useTailnet();
  const client = useQueryClient();
  const commandError = useCommandError();
  const [mode, setMode] = useState<"oauth" | "manual">(settings.mode === "manual" || settings.customControl ? "manual" : "oauth");
  const [clientId, setClientId] = useState(settings.clientId ?? "");
  const [secret, setSecret] = useState("");
  const [replacing, setReplacing] = useState(false);
  const [open, setOpen] = useState(false);
  const [reset, setReset] = useState(false);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState("");
  // Status counts hosts only while enabled; the reset dialog counts tailnet-route hosts from the shared catalogue.
  const routeHosts = useHosts().data?.filter((host) => host.networkRoute === "tailnet").length;
  if (!api || !status) return null;
  const mtls = status.routeCode === "tailnet.mtls-unsupported";
  const rejected = status.provisioning.state === "rejected" || (status.connection.code === "tailnet.credential-rejected" || status.connection.code === "tailnet.tags-not-permitted");
 const tagRefusal = status.connection.code === "tailnet.tags-not-permitted" ? status.connection : status.provisioning.code === "tailnet.tags-not-permitted" ? status.provisioning : undefined;
  const connected = status.connection.state === "connected";
  const expanded = setup || open || rejected || !connected;
  const sameMode = mode === settings.mode;
  const stored = sameMode && settings.secretConfigured;
  const ready = mode === "oauth" ? !!clientId.trim() && (!!secret || sameMode && settings.secretConfigured) : !!secret || sameMode && settings.secretConfigured;
  const response = status.connection.state === "control-unreachable" || status.connection.state === "joining" && status.connection.code === "tailnet.coordination-unreachable" ? "unreachable" : status.connection.state === "connected" && !status.routeAvailable ? "connectedNoRoute" : status.connection.state;
  async function save(enabled: boolean) {
    if (!api) return;
    setBusy(true); setFeedback("");
    try {
      const saved = await api.save(enabled ? { enabled, mode, ...(mode === "oauth" ? { clientId: clientId.trim(), clientSecret: secret } : { authKey: secret }) } : { enabled: false });
      setSecret(""); setReplacing(false); client.setQueryData(["tailnet-settings"], saved); setFeedback(t("saved"));

    } catch (error) { setFeedback(commandError(error)); }
    finally { setSecret(""); setBusy(false); }
  }
  return <section data-slot="tailnet-connection" className="min-w-0 max-w-full overflow-hidden rounded-xl border bg-card/40">
    <header className="flex min-w-0 gap-3 border-b p-4">
      <Network className="mt-1 size-5 shrink-0 text-primary-text" aria-hidden />
      <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{t("name")}</h3><ToneBadge tone={connected ? "ok" : status.connection.state === "control-unreachable" ? "warn" : "gray"}>{connectionLabel}</ToneBadge>{rejected && <ToneBadge tone="err">{wire("provisioning.rejected")}</ToneBadge>}</div>
        <p role="status" className="mt-1 text-sm text-muted-foreground break-words">{t(response, { tailnet: status.tailnetName ?? status.hubAddress ?? "…" })}</p>
      </div>
    </header>
    {mtls && <p role="alert" className="border-b p-4 text-sm text-warning">{t("mtls")}</p>}
    {rejected && <div role="alert" className="border-b p-4 text-sm text-destructive"><p>{t(tagRefusal ? "tags-not-permitted" : connected ? "rejected" : "rejectedDisconnected")}</p>{tagRefusal?.refusedTags?.length ? <p className="font-mono">{t("steps.refusedTags", { tags: tagRefusal.refusedTags.join(", ") })}</p> : null}<p>{t("steps.joinRetry")}</p>{status.provisioning.providerStatus && <p>{t("providerStatus", { status: status.provisioning.providerStatus })}</p>}</div>}
    {status.provisioning.state === "checking" && <p role="status" className="p-4 text-sm">{t("checking")}</p>}
    {status.provisioning.state === "unavailable" && <p role="status" className="border-b p-4 text-sm">{wire("codes.provider-unavailable")}</p>}
    {(connected || status.connection.state === "control-unreachable") && <dl className="grid min-w-0 gap-2 border-b p-4 text-sm sm:grid-cols-[10rem_minmax(0,1fr)]">
      <dt className="text-muted-foreground">{t("tailnet")}</dt><dd className="break-all">{status.tailnetName ?? "–"}</dd>
      <dt className="text-muted-foreground">{t("hub")}</dt><dd className="font-mono break-all">{settings.hostname} · {status.hubAddress ?? "–"}</dd>
      <dt className="text-muted-foreground">{t("registration")}</dt><dd>{t(status.mode === "manual" ? "manual" : "oauth")}</dd>
      <dt className="text-muted-foreground">{t("hosts")}</dt><dd>{t("counts", { count: status.hostCount, online: status.onlineHosts })}</dd>
    </dl>}
    {status.connection.state === "joining" && <ul className="grid gap-2 border-b p-4 text-sm" aria-label={t("checking")}>
      <li>{t("joinCredential")} · {t(status.provisioning.state === "ok" || status.provisioning.state === "not-applicable" ? "done" : "pending")}</li><li>{t("joinNode")} · {t("pending")}</li><li>{t("joinAddress")} · {t("pending")}</li>
    </ul>}
    {!setup && connected && !rejected && <Button variant="ghost" className="min-h-10 w-full justify-start rounded-none px-4" aria-expanded={expanded} onClick={() => setOpen(!open)}>{t("accessDetails")}</Button>}
    {expanded && <fieldset disabled={mtls || busy} className="flex min-w-0 flex-col gap-5 p-4 [&_input]:min-h-10">
      <ChoiceCards value={mode} onChange={(next) => { setMode(next); setSecret(""); setReplacing(false); }} label={t("registration")} options={[
        { value: "oauth", title: t("oauth"), description: t("oauthHint"), badge: <ToneBadge tone="accent">{t("recommended")}</ToneBadge> },
        { value: "manual", title: t("manual"), description: t("manualHint") },
      ]} />
      <PolicyStep settings={settings} done={connected && !rejected ? "setUp" : undefined} onCopied={setFeedback} />
      <CredentialStep settings={settings} mode={mode} done={stored && !rejected ? "setUp" : undefined} onCopied={setFeedback} />
      <Step title={t("paste")} done={stored && status.enabled ? "setUp" : undefined}>{mode === "oauth" && <TextField id="tailnet-client-id" label={t("clientId")} value={clientId} onChange={setClientId} />}
        {stored && !replacing ? <div className="flex flex-col gap-2">
          <label htmlFor="tailnet-secret" className="text-sm font-medium">{t(mode === "oauth" ? "credentialLabel" : "authKey")}</label>
          <div className="flex flex-wrap items-center gap-2"><Input id="tailnet-secret" readOnly value="••••••••" aria-describedby="tailnet-secret-saved" className="min-w-0 flex-1" /><span id="tailnet-secret-saved" className="text-sm">{t("savedValue")}</span><Button type="button" variant="outline" onClick={() => { setSecret(""); setReplacing(true); }}>{t("replaceValue")}</Button></div>
          <p className="text-sm text-muted-foreground">{t("credentialHint")}</p>
        </div> : <TextField id="tailnet-secret" type="password" autoComplete="new-password" label={t(mode === "oauth" ? "credentialLabel" : "authKey")} value={secret} onChange={setSecret} description={t("credentialHint")} trailing={stored ? <Button type="button" variant="outline" onClick={() => { setSecret(""); setReplacing(false); }}>{t("cancelReplace")}</Button> : undefined} />}
      </Step>
      <Button className="min-h-10 self-start" disabled={!ready || busy || mtls || settings.customControl && mode === "oauth"} onClick={() => void save(true)}>{t(setup || !status.enabled ? "connect" : "save")}</Button>
    </fieldset>}
    {feedback && <p role="status" className="px-4 pb-4 text-sm">{feedback}</p>}
    {status.enabled && <div className="border-t p-4"><TestStep api={api} saved={!!settings.secretConfigured} revision={status.credentialRevision} manual={settings.mode === "manual"} /></div>}
    <footer className="flex flex-wrap items-center gap-2 border-t p-4">
      {onSkip && !status.enabled && <Button variant="outline" className="min-h-10" onClick={onSkip}>{t("skip")}</Button>}
      {setup && status.enabled && <Button className="min-h-10" onClick={onContinue}>{t("continue")}</Button>}
      {!setup && status.enabled && <Button variant="outline" className="min-h-10" disabled={busy} onClick={() => void save(false)}>{t("disable")}</Button>}
      {!setup && <Button variant="ghost" className="ml-auto min-h-10 text-destructive" disabled={busy} onClick={() => setReset(true)}>{t("reset")}</Button>}
    </footer>
    <ConfirmDialog contentClassName="[&_button]:min-h-10" open={reset} onOpenChange={setReset} title={t("resetTitle")} description={<p className="break-words">{t("resetBody", { count: routeHosts ?? status.hostCount, hostname: settings.hostname })}</p>} confirmLabel={t("resetConfirm")} destructive onConfirm={async () => { await api.reset(); await client.invalidateQueries({ queryKey: ["tailnet-settings"] }); }} />
  </section>;
}
