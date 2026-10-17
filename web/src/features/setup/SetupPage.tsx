import { ArrowLeft, ArrowRight, CheckCircle2, LayoutDashboard, Pencil, RotateCcw, Server, Settings2 } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { errorCode } from "@/data/api";
import { useAdapter } from "@/data/use-adapter";
import { useAuthMode, useCurrentUser } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { DesignLabPanel } from "@/runtime";
import { useMotionEnabled, useTokens } from "@/shared/theme/use-design-tokens";
import { onboardingStore } from "@/features/hosts/onboarding/onboarding-store";
import { useAuthErrors, useGeneralErrors, useHostDefaultErrors } from "@/features/settings/field-rules";
import { AuthModeFields } from "@/features/settings/sections/AuthModeFields";
import { GeneralFields } from "@/features/settings/sections/GeneralSection";
import { HostDefaultsFields } from "@/features/settings/sections/SimpleSections";
import { BrandLogo } from "@/shared/brand/BrandLogo";
import { TextField } from "@/shared/components/form-field";
import { StepRail } from "@/shared/components/step-rail";
import { useAdapterText } from "@/data/use-adapter-text";
import { cn } from "@/shared/lib/utils";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Toaster } from "@/shared/ui/sonner";
import { Spinner } from "@/shared/ui/spinner";
import { DotField } from "@/shared/visual/DotField";

import { HubAddressDiscovery } from "./HubAddressDiscovery";
import { READABLE_PLATE } from "./readable-plate";
import { applyDiscovery, patchSetup, restartSetup, SETUP_STEPS, setupStore, useSetupDraft, type SetupStep } from "./setup-draft";

function ReviewBlock({ title, onEdit, rows }: { title: string; onEdit: () => void; rows: [string, ReactNode][] }) {
  const t = useTranslations("setup");
  return (
    <section className="rounded-xl border bg-card/40 p-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{title}</h3>
        <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={onEdit} aria-label={`${t("edit")}: ${title}`}>
          <Pencil data-icon="inline-start" />
          {t("edit")}
        </Button>
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

/** First-run draft stays in memory while the operator moves between steps. */
export default function SetupPage() {
  const t = useTranslations("setup");
  const tr = useTranslations("setup.review");
  const dt = useAdapterText();
  const adapter = useAdapter();
  const mode = useAuthMode();
  const currentUser = useCurrentUser();
  const queryClient = useQueryClient();
  const tokens = useTokens();
  const motion = useMotionEnabled();
  const draft = useSetupDraft();
  const [params, setParams] = useSearchParams();
  const [attempted, setAttempted] = useState<SetupStep[]>([]);
  const [busy, setBusy] = useState(false);
  const general = useGeneralErrors(draft.general);
  const hostErr = useHostDefaultErrors(draft.hostDefaults);
  const authErr = useAuthErrors(draft.auth, draft.password);
  const connected = adapter.kind === "http";
  const signedInUser = currentUser.data;
  useEffect(() => {
    if (!connected || !mode.data || !signedInUser) return;
    setupStore.set((state) => ({ ...state, auth: { ...state.auth, mode: mode.data, local: { username: signedInUser.username, passwordSet: !signedInUser.requiresPasswordChange } } }));
  }, [connected, mode.data, signedInUser]);
  const discovery = useQuery({ queryKey: ["hub-discovery"], queryFn: () => adapter.discoverHub(), staleTime: Infinity, retry: false });
  const unset = draft.hubAddressOrigin === null;
  // Prefill once data is there and again after a replay; typed values are never replaced.
  useEffect(() => {
    if (discovery.data && unset) setupStore.set((d) => applyDiscovery(d, discovery.data));
  }, [discovery.data, unset]);

  const requested = params.get("step") as SetupStep | null;
  const index = Math.min(Math.max(0, SETUP_STEPS.indexOf(requested ?? "hub")), draft.reached);
  const step = SETUP_STEPS[index] ?? "hub";
  const errorCount = { hub: [general.hubName, general.hubAddress, general.domainUrl], projects: Object.values(hostErr), auth: [...Object.values(authErr), ...(connected && draft.auth.mode === "local" && !draft.auth.local.passwordSet ? [draft.currentPassword ? undefined : t("currentPasswordRequired")] : [])], review: [], done: [] }[step].filter(Boolean).length;
  const go = (s: SetupStep) => setParams({ step: s });

  const next = async () => {
    if (errorCount) {
      setAttempted((a) => [...new Set([...a, step])]);
      return;
    }
    if (step === "review") {
      setBusy(true);
      try {
        if (connected && draft.auth.mode === "local" && !draft.auth.local.passwordSet) {
          await adapter.changePassword(draft.currentPassword, draft.password.password);
          patchSetup({ auth: { ...draft.auth, local: { ...draft.auth.local, passwordSet: true } }, currentPassword: "", password: { password: "", confirm: "" } });
          queryClient.setQueryData(queryKeys.currentUser, { ...currentUser.data, requiresPasswordChange: false });
        }
        const passwordSet = draft.auth.mode === "local" && draft.password.password.length > 0;
        await adapter.completeSetup({ general: draft.general, hostDefaults: draft.hostDefaults, auth: { ...draft.auth, local: { ...draft.auth.local, passwordSet: connected ? true : passwordSet } } });
        patchSetup({ password: { password: "", confirm: "" }, reached: SETUP_STEPS.length - 1 });
        go("done");
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        toast.error(message.startsWith("settings.write.") ? t("saveError") : errorCode(e) ?? dt(message));
      } finally {
        setBusy(false);
      }
      return;
    }
    const n = index + 1;
    patchSetup({ reached: Math.max(draft.reached, n) });
    go(SETUP_STEPS[n] ?? "review");
  };

  const steps = SETUP_STEPS.map((id) => ({ id, title: t(`steps.${id}.title`), description: t(`steps.${id}.description`) }));
  const show = attempted.includes(step);
  const leads: Partial<Record<SetupStep, string>> = { hub: t("hubLead"), projects: t("projectsLead"), auth: t("authLead"), review: t("reviewLead") };

  return (
    <div className="dot-grid min-h-svh">
      <DotField fixed />
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-6 md:px-8 md:py-10">
        <header className={cn(READABLE_PLATE, "flex flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3")}>
          <BrandLogo size={44} entranceSlot="setup" motion={motion} className="shrink-0" />
          <div className="mr-auto">
            <ToneBadge tone="violet" className="mb-1">
              {t(connected ? "badgeConnected" : "badge")}
            </ToneBadge>
            <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
          </div>
          <LanguageSwitch size="xs" />
          {!connected && <Button asChild variant="outline" size="sm">
            <Link to="/dashboard">
              <ArrowLeft data-icon="inline-start" />
              {t("exit")}
            </Link>
          </Button>}
        </header>
        <p className={cn(READABLE_PLATE, "max-w-3xl self-start px-4 py-2.5 text-sm text-muted-foreground")}>{t(connected ? "leadConnected" : "lead")}</p>
        <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
          <div className={cn(READABLE_PLATE, "p-1.5")} data-slot="setup-rail">
            <StepRail steps={steps} current={step} reachable={step === "done" ? -1 : draft.reached} onSelect={(id) => go(id as SetupStep)} label={t("railLabel")} />
          </div>
          <Card className="gap-0 overflow-hidden p-0">
            <div className="border-b px-5 py-4">
              <h2 className="text-lg font-semibold tracking-tight">{t(`steps.${step}.title`)}</h2>
              {leads[step] && <p className="text-sm text-muted-foreground">{leads[step]}</p>}
            </div>
            <div key={step} className="flex flex-col gap-4 p-5">
              {step === "hub" && (
                <>
                  <HubAddressDiscovery
                    discovery={discovery}
                    address={draft.general.hubAddress}
                    origin={draft.hubAddressOrigin}
                    onPick={(c) => patchSetup({ general: { ...draft.general, hubAddress: c.address }, hubAddressOrigin: { kind: "discovered", candidateId: c.id } })}
                  />
                  <GeneralFields
                    value={draft.general}
                    onChange={(p) => patchSetup({ general: { ...draft.general, ...p }, ...(p.hubAddress !== undefined ? { hubAddressOrigin: { kind: "manual" as const } } : {}) })}
                    showErrors={show}
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
              {step === "auth" && (connected ? (
                <div className="flex flex-col gap-4">
                  <p className="text-sm">{t("activeMode", { mode: tr(`modes.${draft.auth.mode}`) })}</p>
                  <p className="text-xs text-muted-foreground">{t("modeConfiguredAtStartup")}</p>
                  {draft.auth.mode === "local" && !draft.auth.local.passwordSet && <div className="grid gap-4 sm:grid-cols-2">
                    <TextField id="setup-current-password" type="password" label={t("currentPassword")} value={draft.currentPassword} onChange={(currentPassword) => patchSetup({ currentPassword })} error={show && !draft.currentPassword ? t("currentPasswordRequired") : undefined} autoComplete="current-password" className="sm:col-span-2" />
                    <TextField id="setup-new-password" type="password" label={t("newPassword")} value={draft.password.password} onChange={(password) => patchSetup({ password: { ...draft.password, password } })} error={show ? authErr.password : undefined} autoComplete="new-password" />
                    <TextField id="setup-confirm-password" type="password" label={t("confirmPassword")} value={draft.password.confirm} onChange={(confirm) => patchSetup({ password: { ...draft.password, confirm } })} error={show ? authErr.confirm : undefined} autoComplete="new-password" />
                  </div>}
                </div>
              ) : <AuthModeFields value={draft.auth} onChange={(auth) => patchSetup({ auth })} password={draft.password} onPassword={(password) => patchSetup({ password })} showErrors={show} idPrefix="setup-auth" />)}
              {step === "review" && (
                <div className="flex flex-col gap-3">
                  <ReviewBlock
                    title={tr("hub")}
                    onEdit={() => go("hub")}
                    rows={[
                      [tr("name"), draft.general.hubName],
                      [tr("url"), draft.general.hubAddress],
                      [tr("domain"), draft.general.domainUrl || tr("domainNone")],
                      [tr("tz"), draft.general.timeZone],
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
                  <ReviewBlock
                    title={tr("auth")}
                    onEdit={() => go("auth")}
                    rows={[
                      [tr("mode"), tr(`modes.${draft.auth.mode}`)],
                      ...(draft.auth.mode === "local" ? ([[tr("user"), draft.auth.local.username], [tr("password"), tr(connected ? draft.auth.local.passwordSet ? "passwordAlreadySet" : "passwordWillChange" : "passwordSet")]] as [string, string][]) : []),
                      ...(draft.auth.mode === "proxy" ? ([[tr("header"), draft.auth.proxy.userHeader]] as [string, string][]) : []),
                    ]}
                  />
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
              {step === "done" && (
                <div className="flex flex-col items-start gap-4 py-2">
                  <span className="lum-icon grid size-14 animate-pop-in place-items-center rounded-full text-white [&_svg]:size-7" data-tone="ok">
                    <CheckCircle2 />
                  </span>
                  <div>
                    <h3 className="text-xl font-semibold tracking-tight">{t("done.title", { name: draft.general.hubName })}</h3>
                    <p className="mt-1 max-w-xl text-sm text-muted-foreground">{t(connected ? "done.bodyConnected" : "done.body")}</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {!connected && <Button asChild>
                      <Link to="/hosts/new" onClick={() => onboardingStore.set(null)}>
                        <Server data-icon="inline-start" />
                        {t("done.host")}
                      </Link>
                    </Button>}
                    <Button asChild variant="outline">
                      <Link to="/dashboard" onClick={() => { if (connected) queryClient.setQueryData(queryKeys.setupStatus, true); }}>
                        <LayoutDashboard data-icon="inline-start" />
                        {t("done.dashboard")}
                      </Link>
                    </Button>
                    <Button asChild variant="ghost">
                      <Link to="/settings/general" onClick={() => { if (connected) queryClient.setQueryData(queryKeys.setupStatus, true); }}>
                        <Settings2 data-icon="inline-start" />
                        {t("done.settings")}
                      </Link>
                    </Button>
                    {!connected && (
                      <Button
                        variant="ghost"
                        onClick={() => {
                          restartSetup();
                          setAttempted([]);
                          go("hub");
                        }}
                      >
                        <RotateCcw data-icon="inline-start" />
                        {t("done.replay")}
                      </Button>
                    )}
                  </div>
                </div>
              )}
            </div>
            {step !== "done" && (
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
            )}
          </Card>
        </div>
      </div>
      <Toaster position="top-right" offset={{ top: 24, right: 16 }} theme={tokens.theme} />
      <DesignLabPanel />
    </div>
  );
}
