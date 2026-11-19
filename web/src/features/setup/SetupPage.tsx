import { discoveryAppUrl } from "@/domain/hub-discovery";
import { normalizeDomainUrl } from "@/domain/validation";
import { MODULES } from "@/domain/modules";
import { ModuleChoicesFields } from "@/features/settings/sections/ModuleChoicesFields";
import { ArrowLeft, ArrowRight, CheckCircle2, KeyRound, Pencil, Server } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";

import { useAdapter } from "@/data/use-adapter";
import { useAuthMode, useConnectedSettings, useCurrentUser } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useCommandError } from "@/data/use-command-error";
import { useAuthErrors, useGeneralErrors, useHostDefaultErrors } from "@/features/settings/field-rules";
import { AuthModeFields } from "@/features/settings/sections/AuthModeFields";
import { GeneralFields } from "@/features/settings/sections/GeneralSection";
import { HostDefaultsFields } from "@/features/settings/sections/SimpleSections";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Spinner } from "@/shared/ui/spinner";

import { HubAddressDiscovery } from "./HubAddressDiscovery";
import { SetupCompletion } from "./SetupCompletion";
import { SetupFrame } from "./SetupFrame";
import { applyDiscovery, hydrateFromSettings, patchSetup, SETUP_STEPS, setupStore, useSetupDraft, type SetupStep } from "./setup-draft";

function ReviewBlock({ title, onEdit, rows }: { title: string; onEdit?: () => void; rows: [string, ReactNode][] }) {
  const t = useTranslations("setup");
  return (
    <section className="rounded-xl border bg-card/40 p-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{title}</h3>
        {onEdit && <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={onEdit} aria-label={`${t("edit")}: ${title}`}>
          <Pencil data-icon="inline-start" />
          {t("edit")}
        </Button>}
      </div>
      <dl className="grid gap-x-4 gap-y-1.5 text-sm sm:grid-cols-[11rem_1fr]">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="min-w-0 font-mono text-[13px] break-all">{v}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

/**
 * First-run configuration wizard. On a connected hub the identity claim (SetupClaimPage) already
 * created the administrator, so the access step only summarises it; the draft stays in memory.
 */
export default function SetupPage() {
  const t = useTranslations("setup");
  const tr = useTranslations("setup.review");
  const tm = useTranslations("settings.modules");
  const commandError = useCommandError();
  const adapter = useAdapter();
  const navigate = useNavigate();
  const mode = useAuthMode();
  const timeZone = useHubTimeZone();
  const currentUser = useCurrentUser();
  const queryClient = useQueryClient();
  const draft = useSetupDraft();
  const [params, setParams] = useSearchParams();
  const [attempted, setAttempted] = useState<SetupStep[]>([]);
  const [busy, setBusy] = useState(false);
  const [completing, setCompleting] = useState(false);

  const hostErr = useHostDefaultErrors(draft.hostDefaults);
  const authErr = useAuthErrors(draft.auth, draft.password);
  const connected = adapter.kind === "http";
  const saved = useConnectedSettings(connected && !draft.hydrated);
  const savedModules = useQuery({ queryKey: ["module-states", "setup"], queryFn: () => adapter.getModuleStates?.() ?? null, enabled: connected && !draft.hydrated, retry: false });
  // Editing and Finish wait until both reads succeeded; a failed read never counts as hydrated.
  useEffect(() => {
    if (connected && saved.isSuccess && savedModules.isSuccess) setupStore.set((d) => hydrateFromSettings(d, saved.data, savedModules.data ?? undefined));
  }, [connected, saved.isSuccess, saved.data, savedModules.isSuccess, savedModules.data]);
  const initializing = connected && !draft.hydrated;
  const discovery = useQuery({ queryKey: ["hub-discovery"], queryFn: () => adapter.discoverHub(), staleTime: Infinity, retry: false });
  const appUrl = discoveryAppUrl(discovery.data);
  const general = useGeneralErrors(draft.general, { appUrl });
  const unset = draft.hubAddressOrigin === null;
  // Prefill when data arrives, after an IP change and after a replay; applyDiscovery fills each field only while untouched.
  useEffect(() => {
    if (discovery.data && (!connected || draft.hydrated)) setupStore.set((d) => applyDiscovery(d, discovery.data));
  }, [discovery.data, unset, connected, draft.hydrated]);

  // The connected access step is complete: the claim created the administrator.
  const reached = connected ? Math.max(1, draft.reached) : draft.reached;
  const requested = params.get("step") as SetupStep | null;
  const index = Math.min(Math.max(0, SETUP_STEPS.indexOf(requested ?? (connected ? "hub" : "access"))), reached);
  const step = SETUP_STEPS[index] ?? "hub";
  const errorCount = { access: connected ? [] : Object.values(authErr), hub: [general.hubName, general.hubAddress, general.domainUrl], projects: Object.values(hostErr), modules: [], review: [] }[step].filter(Boolean).length;
  const go = (s: SetupStep) => setParams({ step: s });

  const next = async () => {
    if (discovery.isPending && draft.general.domainUrl.trim() && !/^https?:\/\//i.test(draft.general.domainUrl.trim()) && (step === "hub" || step === "review")) return;
    if (errorCount || (step === "review" && Object.values(general).some(Boolean))) {
      setAttempted((a) => [...new Set([...a, step, ...(step === "review" ? ["hub" as const] : [])])]);
      if (step === "review") go("hub");
      return;
    }
    if (step === "review") {
      setBusy(true);
      try {
        const passwordSet = connected || (draft.auth.mode === "local" && draft.password.password.length > 0);
        const { local } = draft.auth;
        await adapter.completeSetup({ modules: draft.modules, general: { ...draft.general, hubAddress: draft.general.hubAddress.trim(), domainUrl: normalizeDomainUrl(draft.general.domainUrl, appUrl)! }, hostDefaults: draft.hostDefaults, auth: { ...draft.auth, local: { ...local, passwordSet } } });
        await Promise.all([queryClient.invalidateQueries({ queryKey: ["module-states"] }), queryClient.invalidateQueries({ queryKey: queryKeys.settings }), queryClient.invalidateQueries({ queryKey: queryKeys.connectedSettings })]);
        patchSetup({ password: { password: "", confirm: "" } });
        setCompleting(true);
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        toast.error(message.startsWith("settings.write.") ? t("saveError") : commandError(e));
      } finally {
        setBusy(false);
      }
      return;
    }
    const n = index + 1;
    patchSetup({ reached: Math.max(reached, n) });
    go(SETUP_STEPS[n] ?? "review");
  };

  if (completing) {
    return <SetupCompletion onDone={() => {
      if (connected) queryClient.setQueryData(queryKeys.setupStatus, true);
      void navigate("/dashboard", { replace: true });
    }} />;
  }

  if (initializing) {
    const failed = saved.isError || savedModules.isError;
    return (
      <SetupFrame connected current={step} reachable={0} onSelect={go}>
        <Card className="flex flex-col items-center gap-3 p-8 text-center">
          {failed ? (
            <>
              <p role="alert" className="text-sm text-destructive">{t("savedLoadError")}</p>
              <Button variant="outline" onClick={() => { if (saved.isError) void saved.refetch(); if (savedModules.isError) void savedModules.refetch(); }}>{t("savedRetry")}</Button>
            </>
          ) : (
            <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><Spinner />{t("savedLoading")}</p>
          )}
        </Card>
      </SetupFrame>
    );
  }

  const show = attempted.includes(step);
  const leads: Partial<Record<SetupStep, string>> = { access: t(connected ? "accessLeadConnected" : "authLead"), hub: t("hubLead"), projects: t("projectsLead"), modules: tm("lead"), review: t("reviewLead") };
  const signedIn = currentUser.data;
  const accessRows: [string, string][] = connected
    ? [[tr("mode"), mode.data ? tr(`modes.${mode.data}`) : "…"], [tr("user"), signedIn?.username ?? "…"]]
    : [[tr("mode"), tr(`modes.${draft.auth.mode}`)], ...(draft.auth.mode === "local" ? ([[tr("user"), draft.auth.local.username], [tr("password"), tr("passwordSet")]] as [string, string][]) : []), ...(draft.auth.mode === "proxy" ? ([[tr("header"), draft.auth.proxy.userHeader]] as [string, string][]) : [])];

  return (
    <SetupFrame connected={connected} current={step} reachable={reached} onSelect={go}>
      <Card className="gap-0 overflow-hidden p-0">
        <div className="border-b px-5 py-4">
          <h2 className="text-lg font-semibold tracking-tight">{t(`steps.${step}.title`)}</h2>
          {leads[step] && <p className="text-sm text-muted-foreground">{leads[step]}</p>}
        </div>
        <div key={step} className="flex flex-col gap-4 p-5">
          {step === "access" && (connected ? (
            <div className="flex flex-col gap-3">
              <p className="flex items-center gap-2 text-sm"><KeyRound className="size-4 text-muted-foreground" aria-hidden />{t("accessDone", { user: signedIn?.username ?? "", mode: mode.data ? tr(`modes.${mode.data}`) : "" })}</p>
              <p className="text-xs text-muted-foreground">{t("accessLater")}</p>
            </div>
          ) : <AuthModeFields value={draft.auth} onChange={(auth) => patchSetup({ auth })} password={draft.password} onPassword={(password) => patchSetup({ password })} showErrors={show} idPrefix="setup-auth" />)}
          {step === "hub" && (
            <>
              <HubAddressDiscovery
                discovery={discovery}
                address={draft.general.hubAddress}
                domain={draft.general.domainUrl}
                origin={draft.hubAddressOrigin}
                onPick={(c) => patchSetup(c.addressKind === "ip" ? { general: { ...draft.general, hubAddress: c.address }, hubAddressOrigin: { kind: "discovered", candidateId: c.id } } : { general: { ...draft.general, domainUrl: c.address }, domainEdited: true })}
              />
              <GeneralFields
                value={{ ...draft.general, timeZone }}
                onChange={(p) => patchSetup({ general: { ...draft.general, ...p }, ...(p.hubAddress !== undefined ? { hubAddressOrigin: { kind: "manual" as const } } : {}), ...(p.domainUrl !== undefined ? { domainEdited: true } : {}) })}
                showErrors={show}
                appUrl={appUrl}
                domainPending={discovery.isPending}
                idPrefix="setup-gen"
              />
            </>
          )}
          {step === "projects" && (
            <>
              <HostDefaultsFields value={draft.hostDefaults} onChange={(p) => patchSetup({ hostDefaults: { ...draft.hostDefaults, ...p } })} showErrors={show} idPrefix="setup-hd" />
              <p className="text-xs text-muted-foreground">{t("projectsNote")}</p>
            </>
          )}
          {step === "modules" && <ModuleChoicesFields value={draft.modules} onChange={(modules) => patchSetup({ modules })} />}
          {step === "review" && (
            <div className="flex flex-col gap-3">
              <ReviewBlock title={tr("auth")} onEdit={connected ? undefined : () => go("access")} rows={accessRows} />
              <ReviewBlock
                title={tr("hub")}
                onEdit={() => go("hub")}
                rows={[
                  [tr("name"), draft.general.hubName],
                  [tr("url"), draft.general.hubAddress],
                  [tr("domain"), draft.general.domainUrl || tr("domainNone")],
                  [tr("tz"), timeZone],
                ]}
              />
              <ReviewBlock
                title={tr("projects")}
                onEdit={() => go("projects")}
                rows={[
                  [tr("project"), draft.hostDefaults.projectDir],
                  [tr("install"), draft.hostDefaults.installDir],
                  [tr("ids"), draft.hostDefaults.puid && draft.hostDefaults.pgid ? `${draft.hostDefaults.puid}:${draft.hostDefaults.pgid}` : tr("idsAuto")],
                  [tr("socket"), draft.hostDefaults.detectSocketGroup ? tr("socketAuto") : tr("socketManual")],
                ]}
              />
              <ReviewBlock title={tm("title")} onEdit={() => go("modules")} rows={MODULES.map((m) => [tm(`names.${m.id}`), tm(draft.modules[m.id] ? "on" : "off")])} />
              <section className="rounded-xl border bg-card/40 p-4">
                <h3 className="mb-2 text-sm font-semibold">{tr("connection")}</h3>
                <ul className="flex flex-col gap-1.5 text-sm">
                  <li className="flex gap-2">
                    <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" aria-hidden />
                    {tr("urlCheck")}
                  </li>
                  <li className="flex gap-2">
                    <Server className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
                    {tr("hosts")}: {tr("hostsNone")}
                  </li>
                </ul>
              </section>
            </div>
          )}
        </div>
        <footer className="flex flex-wrap items-center gap-2 border-t bg-muted/20 px-5 py-3">
          <Button variant="outline" disabled={index === 0} onClick={() => go(SETUP_STEPS[index - 1] ?? "hub")}>
            <ArrowLeft data-icon="inline-start" />
            {t("back")}
          </Button>
          <span className="flex-1" />
          {show && errorCount > 0 && (
            <span role="alert" className="text-xs text-destructive">
              {t("fixErrors", { count: errorCount })}
            </span>
          )}
          <Button onClick={() => void next()} disabled={busy}>
            {busy && <Spinner data-icon="inline-start" />}
            {step === "review" ? (busy ? t("finishing") : t("finish")) : t("next")}
            {!busy && step !== "review" && <ArrowRight data-icon="inline-end" />}
          </Button>
        </footer>
      </Card>
    </SetupFrame>
  );
}
